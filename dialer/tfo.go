package dialer

import (
	"context"
	"net"
	"os"
	"sync"
	"time"

	"github.com/QTraffics/qnetwork/addrs"
	"github.com/metacubex/tfo-go"
)

type TFOConn struct {
	dialer      *tfo.Dialer
	ctx         context.Context
	network     string
	destination addrs.Socksaddr
	conn        net.Conn
	create      chan struct{}
	done        chan struct{}
	access      sync.Mutex
	closeOnce   sync.Once
	err         error
}

func (c *TFOConn) Read(b []byte) (n int, err error) {
	if c.conn == nil {
		select {
		case <-c.create:
			if c.err != nil {
				return 0, c.err
			}
		case <-c.done:
			return 0, os.ErrClosed
		}
	}
	return c.conn.Read(b)
}

func (c *TFOConn) Write(b []byte) (n int, err error) {
	if c.conn != nil {
		return c.conn.Write(b)
	}
	c.access.Lock()
	defer c.access.Unlock()
	select {
	case <-c.create:
		if c.err != nil {
			return 0, c.err
		}
		return c.conn.Write(b)
	case <-c.done:
		return 0, os.ErrClosed
	default:
	}
	conn, err := c.dialer.DialContext(c.ctx, c.network, c.destination.String(), b)
	if err != nil {
		c.err = err
	} else {
		c.conn = conn
	}
	n = len(b)
	close(c.create)
	return
}

func (c *TFOConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			c.conn.Close()
		}
	})
	return nil
}

func (c *TFOConn) LocalAddr() net.Addr {
	if c.conn == nil {
		return addrs.Socksaddr{}
	}
	return c.conn.LocalAddr()
}

func (c *TFOConn) RemoteAddr() net.Addr {
	if c.conn == nil {
		return addrs.Socksaddr{}
	}
	return c.conn.RemoteAddr()
}

func (c *TFOConn) SetDeadline(t time.Time) error {
	if c.conn == nil {
		return os.ErrInvalid
	}
	return c.conn.SetDeadline(t)
}

func (c *TFOConn) SetReadDeadline(t time.Time) error {
	if c.conn == nil {
		return os.ErrInvalid
	}
	return c.conn.SetReadDeadline(t)
}

func (c *TFOConn) SetWriteDeadline(t time.Time) error {
	if c.conn == nil {
		return os.ErrInvalid
	}
	return c.conn.SetWriteDeadline(t)
}

func (c *TFOConn) UnderlayConn() net.Conn {
	return c.conn
}

func (c *TFOConn) NeedHandshake() bool {
	return c.conn == nil
}

func (c *TFOConn) Handshake(bs []byte) (int, error) {
	return c.Write(bs)
}
