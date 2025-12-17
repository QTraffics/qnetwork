package netio

import (
	"net"

	"github.com/qtraffics/qtfra/buf"
	"github.com/qtraffics/qtfra/enhancements/pool"
)

type PacketWriter interface {
	WriteTo(bs []byte, destination net.Addr) (n int, err error)
}

type BindPacketWriter struct {
	PacketWriter

	Destination net.Addr
}

func (p *BindPacketWriter) WriteTo(bs []byte, destination net.Addr) (n int, err error) {
	return p.PacketWriter.WriteTo(bs, p.Destination)
}

type UDPPacket struct {
	Buf *buf.Buffer
	OOB []byte
}

var packetPool = pool.New[UDPPacket](func() UDPPacket {
	return UDPPacket{}
})

func NewPacket(p *buf.Buffer, oob []byte) UDPPacket {
	pp := packetPool.Get()
	pp.Buf = p
	pp.OOB = oob
	return pp
}

func PutPacket(p UDPPacket) {
	p.Buf.Free()
	p.Buf = nil
	p.OOB = nil
	packetPool.Put(p)
}
