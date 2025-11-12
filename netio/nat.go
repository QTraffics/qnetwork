package netio

import (
	"net"

	"github.com/qtraffics/qnetwork/addrs"
)

func NewBidirectionalNatConn(conn net.PacketConn, destination addrs.Socksaddr, source addrs.Socksaddr) *BidNatPacketConn {
	return &BidNatPacketConn{
		PacketConn:  conn,
		destination: destination.NoPort(),
		source:      source.NoPort(),
	}
}

var _ net.PacketConn = (*BidNatPacketConn)(nil)

type BidNatPacketConn struct {
	net.PacketConn

	destination addrs.Socksaddr
	source      addrs.Socksaddr
}

func (b *BidNatPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = b.PacketConn.ReadFrom(p)
	if err != nil {
		return n, addr, err
	}
	originalSource := addrs.FromNetAddr(addr)
	if originalSource.NoPort() == b.source {
		originalSource = addrs.Socksaddr{
			Fqdn: b.destination.Fqdn,
			Addr: b.destination.Addr,
			Port: originalSource.Port,
		}
	}

	addr = originalSource.UDPAddr()
	return n, addr, err
}

func (b *BidNatPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	originalDestination := addrs.FromNetAddr(addr)
	if originalDestination.NoPort() == b.destination {
		originalDestination = addrs.Socksaddr{
			Fqdn: b.source.Fqdn,
			Addr: b.source.Addr,
			Port: originalDestination.Port,
		}
	}
	return b.PacketConn.WriteTo(p, originalDestination.UDPAddr())
}
