package dialer

import (
	"context"
	"net"
	"net/netip"

	"github.com/QTraffics/qnetwork/addrs"
	"github.com/QTraffics/qnetwork/meta"
)

type Dialer interface {
	DialContext(ctx context.Context, network meta.Network, address addrs.Socksaddr) (net.Conn, error)
	ListenPacket(ctx context.Context, address addrs.Socksaddr) (net.PacketConn, error)
}

type ParallelDialer interface {
	Dialer
	DialParallel(ctx context.Context, network meta.Network, address []netip.Addr, port uint16) (net.Conn, error)
}
