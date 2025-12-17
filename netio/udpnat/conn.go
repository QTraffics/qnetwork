package udpnat

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/netio"
	"github.com/qtraffics/qnetwork/netio/pipe"
)

type SetPacketHandler interface {
	SetHandler(h PacketHandler)
}

type Conn interface {
	net.Conn
	net.PacketConn

	SetPacketHandler
}

type PacketHandler interface {
	NewPacket(packet2 netio.UDPPacket)
}

type PacketHandlerFunc func(p netio.UDPPacket)

func (o PacketHandlerFunc) NewPacket(p netio.UDPPacket) { o(p) }

var _ Conn = (*natConn)(nil)

type natConn struct {
	source       addrs.Socksaddr
	destination  addrs.Socksaddr
	writer       netio.PacketWriter
	closeOnce    sync.Once
	closeChan    chan struct{}
	onClose      func(c Conn)
	readDeadline pipe.Deadline

	packets chan netio.UDPPacket

	handler atomic.Pointer[PacketHandler]
}

func (c *natConn) SetHandler(h PacketHandler) {
	c.handler.Store(&h)
	if c.packets == nil {
		return
	}
fetch:
	for {
		select {
		case pack := <-c.packets:
			h.NewPacket(pack)
			netio.PutPacket(pack)
		default:
			break fetch
		}
	}
}

func (c *natConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.Read(p)
	return n, c.source.UDPAddr(), err
}

func (c *natConn) WriteTo(p []byte, destination net.Addr) (n int, err error) {
	return c.writer.WriteTo(p, destination)
}

func (c *natConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		if c.onClose != nil {
			c.onClose(c)
		}
	})
	return nil
}

func (c *natConn) RemoteAddr() net.Addr {
	return c.source.UDPAddr()
}

func (c *natConn) LocalAddr() net.Addr {
	return c.destination.UDPAddr()
}

func (c *natConn) SetDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *natConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *natConn) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *natConn) Read(p []byte) (n int, err error) {
	select {
	case pack := <-c.packets:
		defer netio.PutPacket(pack)
		// _ = pack.oob // discard oob
		// the caller should make sure the p is bigger enough that it can accept all the message.
		n, err = pack.Buf.Read(p[:])

		return n, err
	case <-c.closeChan:
		return 0, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		// https://go-review.googlesource.com/c/go/+/546275
		return 0, context.DeadlineExceeded
	}
}

func (c *natConn) Write(p []byte) (n int, err error) {
	return c.writer.WriteTo(p, c.source.UDPAddr())
}

func (c *natConn) isClose() bool {
	select {
	case <-c.closeChan:
		return true
	default:
		return false
	}
}
