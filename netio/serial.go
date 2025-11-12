package netio

import (
	"context"
	"net"
	"net/netip"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/dialer"
	"github.com/qtraffics/qtfra/ex"
)

func ListenPacketSerial(ctx context.Context, dialer dialer.Dialer, destination []netip.Addr, port uint16) (net.PacketConn, error) {
	errJoin := &ex.JoinError{}
	var (
		packetConn net.PacketConn
		err        error
	)
	for _, addr := range destination {
		dialAddr := addrs.FromAddrPort(netip.AddrPortFrom(addr, port))
		packetConn, err = dialer.ListenPacket(ctx, dialAddr)
		if packetConn != nil {
			return packetConn, nil
		}
		errJoin.NewError(err)
	}
	return nil, errJoin.Err
}
