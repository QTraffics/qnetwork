package dialer

import (
	"cmp"
	"context"
	"net"
	"net/netip"
	"time"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/control"
	"github.com/qtraffics/qnetwork/meta"
	"github.com/qtraffics/qnetwork/netvars"
	"github.com/qtraffics/qtfra/ex"

	"github.com/metacubex/tfo-go"
)

var System = NewDefault()

var _ ParallelDialer = (*DefaultDialer)(nil)

// DefaultDialer Only support dial tcp and udp Protocol.
// only can use to dial ip address ,not support resolve fqdn to ip address.
type DefaultDialer struct {
	dialer4    tfo.Dialer
	dialer6    tfo.Dialer
	udpDialer4 net.Dialer
	udpDialer6 net.Dialer

	udpAddr4 string
	udpAddr6 string

	udpListener net.ListenConfig
}

func (d *DefaultDialer) DialParallel(ctx context.Context, network meta.Network, address []netip.Addr, port uint16) (net.Conn, error) {
	return DialParallel(ctx, d, network, address, port, meta.StrategyDefault, netvars.DefaultDialerFallbackDelay)
}

func (d *DefaultDialer) DialContext(ctx context.Context, network meta.Network, address addrs.Socksaddr) (net.Conn, error) {
	if !address.Dialable() {
		return nil, addrs.ErrNotDialable
	}
	if !address.FqdnOnly() {
		return nil, addrs.ErrAddressNotResolved
	}
	address = address.Unwrap()
	if address.Addr.Is4() && network.Version == meta.NetworkVersion4 ||
		address.Addr.Is6() && network.Version == meta.NetworkVersion6 {
		return nil, ex.New("no address to dialer")
	}

	switch network.Protocol {
	case meta.ProtocolTCP:
		realDialer := d.dialer6
		if address.Addr.Is4() {
			realDialer = d.dialer4
		}
		if realDialer.DisableTFO {
			return realDialer.Dialer.DialContext(ctx, network.String(), address.String())
		}
		return &TFOConn{
			dialer:      &realDialer,
			ctx:         ctx,
			network:     network.String(),
			destination: address,
			create:      make(chan struct{}),
			done:        make(chan struct{}),
		}, nil
	case meta.ProtocolUDP:
		if address.Addr.Is4() {
			return d.udpDialer4.DialContext(ctx, network.String(), address.String())
		}
		return d.udpDialer6.DialContext(ctx, network.String(), address.String())
	default:
		return nil, ex.New("not supported network: ", network.String())
	}
}

func (d *DefaultDialer) ListenPacket(ctx context.Context, address addrs.Socksaddr) (net.PacketConn, error) {
	if address.Addr.Is6() {
		return d.udpListener.ListenPacket(ctx, meta.NetworkUDP6.String(), d.udpAddr6)
	} else if address.Addr.Is4() && !address.Addr.IsUnspecified() {
		return d.udpListener.ListenPacket(ctx, meta.NetworkUDP4.String(), d.udpAddr4)
	}
	return d.udpListener.ListenPacket(ctx, meta.ProtocolUDP.String(), d.udpAddr4)
}

type Config struct {
	Keepalive    net.KeepAliveConfig
	Timeout      time.Duration
	Interface    string
	BindAddress4 netip.Addr
	BindAddress6 netip.Addr
	FwMark       uint32
	ReuseAddr    bool
	ReusePort    bool

	// tcp
	MPTCP bool
	TFO   bool

	// udp
	UDPFragment bool
}

func NewDefault() *DefaultDialer {
	return NewDefaultConfig(Config{
		Keepalive: net.KeepAliveConfig{
			Enable:   true,
			Idle:     netvars.DefaultTCPKeepAliveInitial,
			Interval: netvars.DefaultTCPKeepAliveInterval,
			Count:    netvars.DefaultTCPKeepAliveProbeCount,
		},
		Timeout: netvars.DefaultDialerTimeout,
	})
}

func NewDefaultConfig(config Config) *DefaultDialer {
	var (
		dialer   net.Dialer
		listener net.ListenConfig
	)

	if config.Interface != "" {
		finder := control.NewDefaultInterfaceFinder()
		bindFunc := control.BindToInterface(finder, config.Interface, -1)
		dialer.Control = control.Append(dialer.Control, bindFunc)
		listener.Control = control.Append(listener.Control, bindFunc)
	}
	if config.FwMark != 0 {
		dialer.Control = control.Append(dialer.Control, control.RoutingMark(config.FwMark))
		listener.Control = control.Append(listener.Control, control.RoutingMark(config.FwMark))
	}
	dialer.Timeout = cmp.Or(config.Timeout, netvars.DefaultDialerTimeout)
	dialer.KeepAliveConfig = config.Keepalive

	if config.ReuseAddr {
		listener.Control = control.Append(listener.Control, control.ReuseAddr())
	}

	if config.ReusePort {
		listener.Control = control.Append(listener.Control, control.ReusePort())
	}

	if !config.UDPFragment {
		dialer.Control = control.Append(dialer.Control, control.DisableUDPFragment())
		listener.Control = control.Append(listener.Control, control.DisableUDPFragment())
	}
	if config.MPTCP {
		dialer.SetMultipathTCP(true)
	}

	var (
		dialer4 = tfo.Dialer{DisableTFO: !config.TFO, Dialer: dialer}
		dialer6 = tfo.Dialer{DisableTFO: !config.TFO, Dialer: dialer}

		udpDialer4 = dialer
		udpDialer6 = dialer

		udpAddr4 string
		udpAddr6 string
	)

	if config.BindAddress4.IsValid() {
		bind := config.BindAddress4
		dialer4.LocalAddr = &net.TCPAddr{IP: bind.AsSlice()}
		udpDialer4.LocalAddr = &net.UDPAddr{IP: bind.AsSlice()}
		udpAddr4 = addrs.FromAddrPort(netip.AddrPortFrom(bind, 0)).String()
	}

	if config.BindAddress6.IsValid() {
		bind := config.BindAddress6
		dialer6.LocalAddr = &net.TCPAddr{IP: bind.AsSlice()}
		udpDialer6.LocalAddr = &net.UDPAddr{IP: bind.AsSlice()}
		udpAddr6 = addrs.FromAddrPort(netip.AddrPortFrom(bind, 0)).String()
	}

	return &DefaultDialer{
		dialer4:     dialer4,
		dialer6:     dialer6,
		udpDialer4:  udpDialer4,
		udpDialer6:  udpDialer6,
		udpAddr4:    udpAddr4,
		udpAddr6:    udpAddr6,
		udpListener: listener,
	}
}

//func (d *DefaultDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
//	switch network {
//	case "udp", "udp4", "udp6", "tcp", "tcp4", "tcp6":
//	default:
//		// fallback to default dialer
//		return d.defaultDialer.DialContext(ctx, network, address)
//	}
//
//	host, port, err := net.SplitHostPort(address)
//	if err != nil {
//		return nil, fmt.Errorf("dialer: split host port failed: %s: %w", address, err)
//	}
//	portNum, err := strconv.ParseUint(port, 10, 16)
//	if err != nil {
//		return nil, fmt.Errorf("dialer: invalid port number: %s: %w", port, err)
//	}
//	if !addrs.IsDomainName(host) {
//		addr, err := netip.ParseAddr(host)
//		if err != nil {
//			return nil, fmt.Errorf("dialer: invalid address: %s: %w", host, err)
//		}
//		return d.DialSerial(ctx, network, []netip.Addr{addr}, uint16(portNum))
//	}
//	a, aaaa, err := d.resolver.Lookup(ctx, host, d.resolveStrategy)
//	if err != nil {
//		return nil, fmt.Errorf("dialer: resolve address for %s failed: %w", address, err)
//	}
//
//	return d.DialParallel(ctx, network, d.resolveStrategy, a, aaaa, uint16(portNum))
//}
//
//func (d *DefaultDialer) DialSerial(ctx context.Context, network string, addresses []netip.Addr, port uint16) (net.Conn, error) {
//	conn, err := d.dialSerial(ctx, network, addresses, port)
//	if err != nil {
//		return nil, fmt.Errorf("dialer: %w", err)
//	}
//	return conn, nil
//}
//
//func (d *DefaultDialer) dialSerial(ctx context.Context, network string, addresses []netip.Addr, port uint16) (net.Conn, error) {
//	if len(addresses) == 0 {
//		return nil, errors.New("no address to dial")
//	}
//	var (
//		nn meta.Network
//		ok bool
//	)
//
//	if nn, ok = meta.ParseNetwork(network); !ok {
//		return nil, fmt.Errorf("invalid network :%s", network)
//	}
//
//	availableAddress := filterAddressByNetwork(nn, addresses)
//	if len(availableAddress) == 0 {
//		return nil, fmt.Errorf("no available address found for network: %s", network)
//	}
//
//	var lastErr error
//	for _, addr := range availableAddress {
//		if common.Done(ctx) {
//			return nil, ctx.Err()
//		}
//		var (
//			target    = netip.AddrPortFrom(addr, port)
//			conn      net.Conn
//			err       error
//			tcpDialer *net.Dialer
//			udpDialer *net.Dialer
//		)
//		switch {
//		case addr.Is4():
//			udpDialer = &d.udpDialer4
//			tcpDialer = &d.dialer4
//		case addr.Is6():
//			udpDialer = &d.udpDialer4
//			tcpDialer = &d.dialer4
//		default:
//			tcpDialer = &d.defaultDialer
//			udpDialer = &d.defaultDialer
//		}
//		switch nn.Protocol {
//		case meta.ProtocolUDP:
//			conn, err = udpDialer.DialContext(ctx, network, target.String())
//		case meta.ProtocolTCP:
//			conn, err = tcpDialer.DialContext(ctx, network, target.String())
//		default:
//			conn, err = d.defaultDialer.DialContext(ctx, network, addr.String())
//		}
//
//		if err == nil {
//			return conn, nil
//		}
//
//		lastErr = err
//	}
//
//	return nil, fmt.Errorf("all addresses failed, last error: %w", lastErr)
//}
//
//func (d *DefaultDialer) DialParallel(ctx context.Context, network string, strategy meta.Strategy,
//	ipv4 []netip.Addr, ipv6 []netip.Addr, port uint16) (net.Conn, error) {
//	if len(ipv4) == 0 && len(ipv6) == 0 {
//		return nil, fmt.Errorf("dialer: no available address to dial")
//	}
//
//	if len(ipv4) == 0 || strategy == meta.StrategyIPv6Only {
//		return d.DialSerial(ctx, network, ipv6, port)
//	}
//	if len(ipv6) == 0 || strategy == meta.StrategyIPv4Only {
//		return d.DialSerial(ctx, network, ipv4, port)
//	}
//
//	// happy eyeball implement
//	type dialResult struct {
//		conn net.Conn
//		err  error
//		ipv6 bool
//	}
//
//	resultChan := make(chan dialResult, 2)
//	dialCtx, cancel := context.WithCancel(ctx)
//	defer cancel()
//
//	firstIsIPv4 := strategy == meta.StrategyPreferIPv4
//	var first, second []netip.Addr
//	if firstIsIPv4 {
//		first, second = ipv4, ipv6
//	} else {
//		first, second = ipv6, ipv4
//	}
//
//	go func() {
//		conn, err := d.DialSerial(dialCtx, network, first, port)
//		select {
//		case resultChan <- dialResult{conn: conn, err: err,
//			ipv6: !firstIsIPv4}:
//		case <-dialCtx.Done():
//			if conn != nil {
//				conn.Close()
//			}
//		}
//	}()
//
//	// happy eyeball
//	firstTimer := time.NewTimer(300 * time.Millisecond)
//	defer firstTimer.Stop()
//
//	var secondStarted bool
//	var resultsReceived int
//
//	for resultsReceived < 2 {
//		select {
//		case <-dialCtx.Done():
//			return nil, dialCtx.Err()
//
//		case <-firstTimer.C:
//			if !secondStarted {
//				secondStarted = true
//				go func() {
//					conn, err := d.DialSerial(dialCtx, network, second, port)
//					select {
//					case resultChan <- dialResult{conn: conn, err: err, ipv6: firstIsIPv4}:
//					case <-dialCtx.Done():
//						if conn != nil {
//							conn.Close()
//						}
//					}
//				}()
//			}
//
//		case result := <-resultChan:
//			resultsReceived++
//
//			if result.err == nil {
//				cancel()
//				return result.conn, nil
//			}
//
//			if !secondStarted && resultsReceived == 1 {
//				secondStarted = true
//				firstTimer.Stop()
//				go func() {
//					conn, err := d.DialSerial(dialCtx, network, second, port)
//					select {
//					case resultChan <- dialResult{conn: conn, err: err, ipv6: firstIsIPv4}:
//					case <-dialCtx.Done():
//						if conn != nil {
//							conn.Close()
//						}
//					}
//				}()
//			}
//		}
//	}
//
//	return nil, fmt.Errorf("dialer: all parallel dials failed for both IPv4 and IPv6")
//}
//
//func filterAddressByNetwork(network meta.Network, addr []netip.Addr) []netip.Addr {
//	switch {
//	case network.Version == meta.NetworkVersionDual:
//		return common.Filter(addr, func(it netip.Addr) bool {
//			return it.IsValid()
//		})
//	case network.Version == meta.NetworkVersion4:
//		return common.Filter(addr, func(it netip.Addr) bool {
//			return it.IsValid() && it.Is4()
//		})
//	case network.Version == meta.NetworkVersion6:
//		return common.Filter(addr, func(it netip.Addr) bool {
//			return it.IsValid() && it.Is6()
//		})
//	default:
//		return addr
//	}
//}
