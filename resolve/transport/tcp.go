package transport

import (
	"context"
	"encoding/binary"
	"io"
	"net"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/dialer"
	"github.com/qtraffics/qnetwork/meta"
	"github.com/qtraffics/qtfra/buf"

	"github.com/miekg/dns"
)

type TCPTransportOptions struct {
	Dialer dialer.Dialer
}

type TCPTransport struct {
	serverAddr addrs.Socksaddr
	dialer     dialer.Dialer
}

func NewTCPTransport(server addrs.Socksaddr, options TCPTransportOptions) (*TCPTransport, error) {
	if server.Port == 0 {
		server.Port = 53
	}
	realDialer := options.Dialer
	if realDialer == nil {
		realDialer = dialer.System
	}
	t := &TCPTransport{
		serverAddr: server,
		dialer:     realDialer,
	}
	return t, nil
}

func (t *TCPTransport) Exchange(ctx context.Context, message *dns.Msg) (*dns.Msg, error) {
	conn, err := t.dialer.DialContext(ctx, meta.NetworkTCP, t.serverAddr)
	if err != nil {
		return nil, err
	}
	closeCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closeCtx.Done():
		}
	}()
	defer conn.Close()
	_, err = writeMessage(conn, message)
	if err != nil {
		return nil, err
	}
	ansMsg, err := readMessage(conn)
	return ansMsg, err
}

func readMessage(r io.Reader) (*dns.Msg, error) {
	var length uint16
	err := binary.Read(r, binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}
	if length < 10 {
		return nil, dns.ErrShortRead
	}

	buffer := buf.NewSize(int(length) + 1)
	defer buffer.Free()

	_, err = buffer.ReadFull(r, int(length))
	if err != nil {
		return nil, err
	}

	message := new(dns.Msg)
	err = message.Unpack(buffer.Bytes())
	if err != nil {
		return nil, err
	}
	return message, nil
}

func writeMessage(w io.Writer, message *dns.Msg) (int, error) {
	requestLen := message.Len()
	buffer := buf.NewSize(requestLen + 3)
	defer buffer.Free()
	err := binary.Write(buffer, binary.BigEndian, uint16(requestLen))
	if err != nil {
		return 0, err
	}
	rawMessage, err := message.PackBuffer(buffer.FreeBytes())
	if err != nil {
		return 0, err
	}
	buffer.Truncated(2 + len(rawMessage))
	return w.Write(buffer.Bytes())
}

func exchangeTCP(ctx context.Context, conn net.Conn, message *dns.Msg) (*dns.Msg, error) {
	if deadline, ok := ctx.Deadline(); ok && !deadline.IsZero() {
		_ = conn.SetDeadline(deadline)
	}
	var err error
	_, err = writeMessage(conn, message)
	if err != nil {
		return nil, err
	}
	return readMessage(conn)
}
