package listener

import (
	"context"
	"net"
	"net/netip"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/control"
	"github.com/qtraffics/qnetwork/meta"
	"github.com/qtraffics/qnetwork/resolve"
	"github.com/qtraffics/qtfra/enhancements/slicelib"
	"github.com/qtraffics/qtfra/ex"

	"github.com/metacubex/tfo-go"
)

type Options struct {
	// optional
	Family    string
	Interface string
	ReuseAddr bool
	ReusePort bool

	// tcp
	KeepAlive net.KeepAliveConfig
	TFO       bool
	MPTCP     bool

	// udp
	UDPFragment bool
}

type Listener struct {
	options Options
}

func NewListener(options Options) *Listener {
	return &Listener{
		options: options,
	}
}

func ListenTCP(ctx context.Context, address string, port uint16, opt Options) (net.Listener, error) {
	l := NewListener(opt)
	return l.ListenTCP(ctx, address, port)
}

func ListenUDP(ctx context.Context, address string, port uint16, opt Options) (*net.UDPConn, error) {
	l := NewListener(opt)
	return l.ListenUDP(ctx, address, port)
}

func (l *Listener) ListenUDP(ctx context.Context, address string, port uint16) (*net.UDPConn, error) {
	var listenConfig net.ListenConfig

	if l.options.Interface != "" {
		interfaceFinder := control.NewDefaultInterfaceFinder()
		listenConfig.Control = control.Append(listenConfig.Control, control.BindToInterface(interfaceFinder, l.options.Interface, -1))
	}

	if l.options.ReuseAddr {
		listenConfig.Control = control.Append(listenConfig.Control, control.ReuseAddr())
	}
	if l.options.ReusePort {
		listenConfig.Control = control.Append(listenConfig.Control, control.ReusePort())
	}
	if !l.options.UDPFragment {
		listenConfig.Control = control.Append(listenConfig.Control, control.DisableUDPFragment())
	}

	network := meta.Network{Protocol: meta.ProtocolUDP}
	if l.options.Family == meta.NetworkFamily6 {
		network.Version = meta.NetworkVersion6
	} else if l.options.Family == meta.NetworkFamily4 {
		network.Version = meta.NetworkVersion4
	}

	addresses, err := resolveListenAddresses(ctx, network, address, port, resolve.SystemDNSClient, meta.StrategyDefault)
	if err != nil {
		return nil, err
	}

	return ListenUDPSerial(ctx, listenConfig, network, addresses)
}

func (l *Listener) ListenTCP(ctx context.Context, address string, port uint16) (net.Listener, error) {
	var listenConfig net.ListenConfig

	if l.options.Interface != "" {
		interfaceFinder := control.NewDefaultInterfaceFinder()
		listenConfig.Control = control.Append(listenConfig.Control, control.BindToInterface(interfaceFinder, l.options.Interface, -1))
	}

	if l.options.ReuseAddr {
		listenConfig.Control = control.Append(listenConfig.Control, control.ReuseAddr())
	}
	listenConfig.KeepAliveConfig = l.options.KeepAlive

	if l.options.MPTCP {
		listenConfig.SetMultipathTCP(true)
	}
	network := meta.Network{Protocol: meta.ProtocolTCP}
	switch l.options.Family {
	case meta.NetworkFamily4:
		network.Version = meta.NetworkVersion6
	case meta.NetworkFamily6:
		network.Version = meta.NetworkVersion4
	}

	addresses, err := resolveListenAddresses(ctx, network, address, port, resolve.SystemDNSClient, meta.StrategyDefault)
	if err != nil {
		return nil, err
	}

	return ListenTCPSerial(ctx, listenConfig, network, addresses, l.options.TFO)
}

func ListenTCPSerial(ctx context.Context, lc net.ListenConfig, network meta.Network, address []addrs.Socksaddr, enableTFO bool) (net.Listener, error) {
	if !network.IsTCP() {
		return nil, ex.New("ListenTCPSerial: called on a non-tcp network")
	}
	var tfoListener *tfo.ListenConfig
	if enableTFO {
		tfoListener = &tfo.ListenConfig{
			ListenConfig: lc,
		}
	}
	var (
		nl  net.Listener
		err error
	)
	networkString := network.String()
	for _, a := range address {
		if a.FqdnOnly() {
			return nil, ex.New("ListenTCPSerial : listen on a not-resolved address:", a.String())
		}
		if enableTFO && tfoListener != nil {
			nl, err = tfoListener.Listen(ctx, networkString, a.String())
			if err == nil {
				return nl, nil
			}
			continue
		}
		nl, err = lc.Listen(ctx, networkString, a.String())
		if err == nil {
			return nl, nil
		}
	}
	return nil, ex.Cause(err, "ListenTCPSerial: all failed")
}

func ListenUDPSerial(ctx context.Context, lc net.ListenConfig, network meta.Network, address []addrs.Socksaddr) (*net.UDPConn, error) {
	if !network.IsUDP() {
		return nil, ex.New("ListenUDPSerial: called on a non-udp network")
	}
	var (
		pn  net.PacketConn
		err error
	)
	networkString := network.String()
	for _, a := range address {
		if a.FqdnOnly() {
			return nil, ex.New("ListenUDPSerial : listen on a not-resolved address:", a.String())
		}
		if network.Is4() && a.Addr.Is6() || network.Is6() && a.Addr.Is4() {
			continue // skip
		}
		pn, err = lc.ListenPacket(ctx, networkString, a.String())
		if err == nil {
			return pn.(*net.UDPConn), nil
		}
	}
	return nil, ex.Cause(err, "ListenUDPSerial: all failed")
}

func resolveListenAddresses(ctx context.Context, network meta.Network, address string, port uint16, resolver *resolve.Client, strategy meta.Strategy) ([]addrs.Socksaddr, error) {
	var addresses []addrs.Socksaddr
	if sa := addrs.FromParseSocksaddrHostPort(address, port); sa.FqdnOnly() {
		if network.Is4() && network.Is6() {
		} else if network.Is4() {
			strategy = meta.StrategyIPv4Only
		} else if network.Is6() {
			strategy = meta.StrategyIPv6Only
		}

		netipAddresses, err := resolver.Lookup(ctx, sa.AddrString(), strategy)
		if err != nil {
			return nil, ex.Cause(err, "resolve addresses")
		}
		addresses = slicelib.Map(netipAddresses, func(it netip.Addr) addrs.Socksaddr {
			return addrs.FromAddrPort(netip.AddrPortFrom(it, port))
		})
	} else {
		addresses = append(addresses, sa)
	}
	return addresses, nil
}
