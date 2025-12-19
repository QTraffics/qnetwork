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
	Size    uint32
	Timeout time.Duration
}

type PrepareResult struct {
	Success bool

	OnClose      func(conn Conn)
	Handler      PacketHandler
	PacketWriter netio.PacketWriter
}

type PrepareFunc func(source addrs.Socksaddr, destination addrs.Socksaddr, p netio.UDPPacket) PrepareResult

type UdpNat struct {
	cache freelru.Cache[netip.AddrPort, *natConn]

	prepare PrepareFunc
	opt     Option
}

func (u *UdpNat) NewPacket(buffers *buf.Buffer, source addrs.Socksaddr, destination addrs.Socksaddr) (Conn, bool) {
	pack := netio.NewPacket(buffers, nil)
	conn, exist := u.cache.GetAndRefresh(source.AddrPort(), u.opt.Timeout)
	if !exist || conn.isClose() {
		prepare := u.prepare(source, destination, pack)
		if !prepare.Success {
			return nil, false
		}

		conn = &natConn{
			source:       source,
			destination:  destination,
			writer:       prepare.PacketWriter,
			onClose:      prepare.OnClose,
			closeChan:    make(chan struct{}),
			readDeadline: pipe.MakeDeadline(),
		}
		if prepare.Handler != nil {
			conn.SetHandler(prepare.Handler)
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

func New(prepare PrepareFunc, opt *Option) (*UdpNat, error) {
	if prepare == nil {
		return nil, ex.New("prepare func required: ", opt)
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

	udpnat.prepare = prepare
	udpnat.opt = *opt
	return udpnat, nil
}

var hashSeed = maphash.MakeSeed()

func hashNetipAddrPort(ap netip.AddrPort) uint32 {
	return uint32(maphash.Comparable(hashSeed, ap))
}
