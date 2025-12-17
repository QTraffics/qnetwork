package udpnat

import (
	"hash/maphash"
	"net/netip"
	"time"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/netio"
	"github.com/qtraffics/qnetwork/netio/pipe"
	"github.com/qtraffics/qnetwork/netvars"
	"github.com/qtraffics/qtfra/buf"
	"github.com/qtraffics/qtfra/ex"

	"github.com/elastic/go-freelru"
)

type Option struct {
	Prepare PrepareFunc
	Size    uint32

	Timeout        time.Duration
	InitialHandler PacketHandler
}

type PrepareFunc func(source addrs.Socksaddr, destination addrs.Socksaddr, p netio.UDPPacket) (ok bool, onClose func(c Conn))

type UdpNat struct {
	cache freelru.Cache[netip.AddrPort, *natConn]

	packetWriter netio.PacketWriter

	opt *Option
}

func (u *UdpNat) NewPacket(buffers *buf.Buffer, source addrs.Socksaddr, destination addrs.Socksaddr) (Conn, bool) {
	pack := netio.NewPacket(buffers, nil)
	conn, exist := u.cache.GetAndRefresh(source.AddrPort(), u.opt.Timeout)
	if !exist || conn.isClose() {
		var onClose func(c Conn)
		if u.opt.Prepare != nil {
			success, onCloseFn := u.opt.Prepare(source, destination, pack)
			if !success {
				return nil, false
			}
			onClose = onCloseFn
		}

		conn = &natConn{
			source:      source,
			destination: destination,
			writer:      &netio.BindPacketWriter{PacketWriter: u.packetWriter, Destination: source},

			onClose:      onClose,
			closeChan:    make(chan struct{}),
			readDeadline: pipe.MakeDeadline(),
		}
		if u.opt.InitialHandler != nil {
			conn.SetHandler(u.opt.InitialHandler)
		} else {
			conn.packets = make(chan netio.UDPPacket, 64)
		}
		u.cache.Add(source.AddrPort(), conn)
	}
	if h := conn.handler.Load(); h != nil {
		(*h).NewPacket(pack)
	} else {
		select {
		case conn.packets <- pack:
		default:
			pack.Buf.Free()
			netio.PutPacket(pack)
		}
	}

	return conn, !exist
}

func (u *UdpNat) Close() error {
	for _, k := range u.cache.Keys() {
		cc, ok := u.cache.Get(k)
		if ok && !cc.isClose() {
			_ = cc.Close()
		}
	}
	return nil
}

func New(packetWrite netio.PacketWriter, opt *Option) *UdpNat {
	if packetWrite == nil {
		panic("empty packet writer")
	}

	if opt == nil {
		opt = &Option{}
	}
	if opt.Timeout == 0 {
		opt.Timeout = netvars.DefaultUDPKeepAlive
	}
	if opt.Size == 0 {
		opt.Size = netvars.DefaultUDPConnSize
	}

	udpnat := &UdpNat{}
	udpnat.cache = ex.Must0(freelru.NewSharded[netip.AddrPort, *natConn](opt.Size, hashNetipAddrPort))
	udpnat.cache.SetLifetime(opt.Timeout)
	udpnat.cache.SetOnEvict(func(port netip.AddrPort, conn *natConn) {
		_ = conn.Close()
	})
	udpnat.opt = opt
	udpnat.packetWriter = packetWrite
	return udpnat
}

var hashSeed = maphash.MakeSeed()

func hashNetipAddrPort(ap netip.AddrPort) uint32 {
	return uint32(maphash.Comparable(hashSeed, ap))
}
