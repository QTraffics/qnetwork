package transport

import (
	"context"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/QTraffics/qnetwork/addrs"
	"github.com/QTraffics/qnetwork/dialer"
	"github.com/QTraffics/qnetwork/meta"
	"github.com/QTraffics/qtfra/buf"
	"github.com/QTraffics/qtfra/ex"
	"github.com/QTraffics/qtfra/values"
	"github.com/miekg/dns"
)

const maxUDPSize = 1232

type UDPTransportOptions struct {
	Dialer dialer.Dialer
}

var _ Transport = (*UDPTransport)(nil)

type UDPTransport struct {
	tcp    *TCPTransport
	dialer dialer.Dialer

	serverAddr addrs.Socksaddr

	// internal
	udpSize atomic.Int32
	access  sync.Mutex
	conn    *dnsConnection
	done    chan struct{}
}

func NewUDP(server addrs.Socksaddr, options UDPTransportOptions) *UDPTransport {
	server.Port = values.UseDefault(server.Port, 53)
	options.Dialer = values.UseDefaultNil(options.Dialer, dialer.System)

	t := &UDPTransport{
		tcp: &TCPTransport{
			serverAddr: server,
			dialer:     options.Dialer,
		},
		dialer:     options.Dialer,
		serverAddr: server,
	}

	t.udpSize.Add(maxUDPSize)
	return t
}

func (t *UDPTransport) Close() error {
	t.access.Lock()
	defer t.access.Unlock()
	close(t.done)
	t.done = make(chan struct{})
	return nil
}

func (t *UDPTransport) Exchange(ctx context.Context, message *dns.Msg) (*dns.Msg, error) {
	response, needTcp, err := t.exchange(ctx, message)
	if err != nil {
		return nil, err
	}
	if response.Truncated || needTcp {
		if t.tcp != nil {
			return t.tcp.Exchange(ctx, message)
		}
		return nil, ex.New("truncated")
	}
	return response, nil
}

func (t *UDPTransport) exchange(ctx context.Context, message *dns.Msg) (*dns.Msg, bool, error) {
	t.access.Lock()
	if edns0Opt := message.IsEdns0(); edns0Opt != nil {
		if udpSize := int32(edns0Opt.UDPSize()); udpSize > t.udpSize.Load() {
			t.udpSize.Store(udpSize)
			close(t.done)
			t.done = make(chan struct{})
		}
	}

	t.access.Unlock()
	conn, err := t.open(ctx)
	if err != nil {
		return nil, true, err
	}

	buffer := buf.NewSize(1 + message.Len())
	defer buffer.Free()
	exMessage := *message
	messageId := message.Id
	callback := &dnsCallback{
		done: make(chan struct{}),
	}
	conn.access.Lock()
	conn.queryId++
	exMessage.Id = conn.queryId
	conn.callbacks[exMessage.Id] = callback
	conn.access.Unlock()
	defer func() {
		conn.access.Lock()
		delete(conn.callbacks, exMessage.Id)
		conn.access.Unlock()
	}()
	rawMessage, err := exMessage.PackBuffer(buffer.FreeBytes())
	if err != nil {
		return nil, false, err
	}
	_, err = conn.Write(rawMessage)
	if err != nil {
		conn.Close(err)
		return nil, ex.IsMulti(err, syscall.EMSGSIZE), err
	}
	select {
	case <-callback.done:
		callback.message.Id = messageId
		return callback.message, callback.message.Truncated, nil
	case <-conn.done:
		return nil, false, conn.err
	case <-t.done:
		return nil, false, os.ErrClosed
	case <-ctx.Done():
		conn.Close(ctx.Err())
		return nil, false, ctx.Err()
	}
}

func (t *UDPTransport) open(ctx context.Context) (*dnsConnection, error) {
	t.access.Lock()
	defer t.access.Unlock()
	if t.conn != nil {
		select {
		case <-t.conn.done:
		default:
			return t.conn, nil
		}
	}
	conn, err := t.dialer.DialContext(ctx, meta.NetworkUDP, t.serverAddr)
	if err != nil {
		return nil, err
	}
	dnsConn := &dnsConnection{
		Conn:      conn,
		done:      make(chan struct{}),
		callbacks: make(map[uint16]*dnsCallback),
	}
	go t.recvLoop(dnsConn)
	t.conn = dnsConn
	return dnsConn, nil
}

func (t *UDPTransport) recvLoop(conn *dnsConnection) {
	for {
		buffer := buf.NewSize(int(t.udpSize.Load() + 1))
		_, err := buffer.ReadFromOnce(conn)
		if err != nil {
			buffer.Free()
			conn.Close(err)
			return
		}
		var message dns.Msg
		err = message.Unpack(buffer.Bytes())
		buffer.Free()
		if err != nil {
			conn.Close(err)
			return
		}
		conn.access.RLock()
		callback, loaded := conn.callbacks[message.Id]
		conn.access.RUnlock()
		if !loaded {
			continue
		}
		callback.access.Lock()
		select {
		case <-callback.done:
		default:
			callback.message = &message
			close(callback.done)
		}
		callback.access.Unlock()
	}
}

func exchangeUDP(ctx context.Context, conn net.Conn, message *dns.Msg) (*dns.Msg, error) {
	if deadline, ok := ctx.Deadline(); ok && !deadline.IsZero() {
		_ = conn.SetDeadline(deadline)
	}

	requestLen := message.Len()
	buffer := buf.NewSize(requestLen + 1)
	defer buffer.Free()

	rawMessage, err := message.PackBuffer(buffer.FreeBytes())
	if err != nil {
		return nil, err
	}
	_, err = conn.Write(rawMessage)
	if err != nil {
		return nil, err
	}
	readBuffer := buf.New()
	defer readBuffer.Free()

	_, err = readBuffer.ReadFromOnce(conn)
	if err != nil {
		return nil, err
	}
	response := new(dns.Msg)
	err = response.Unpack(readBuffer.Bytes())
	if err != nil {
		return nil, err
	}
	return response, nil
}

type dnsConnection struct {
	net.Conn
	access    sync.RWMutex
	done      chan struct{}
	closeOnce sync.Once
	err       error
	queryId   uint16
	callbacks map[uint16]*dnsCallback
}

func (c *dnsConnection) Close(err error) {
	c.closeOnce.Do(func() {
		c.err = err
		close(c.done)
		c.Conn.Close()
	})
}

type dnsCallback struct {
	access  sync.Mutex
	message *dns.Msg
	done    chan struct{}
}
