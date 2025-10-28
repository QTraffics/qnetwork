package resolve

import (
	"context"
	"net"
	"time"

	"github.com/QTraffics/qnetwork/addrs"
	"github.com/QTraffics/qnetwork/dialer"
	"github.com/QTraffics/qnetwork/meta"
	"github.com/QTraffics/qtfra/ex"
)

var (
	SystemResolveDialer = NewResolverDialerSystem()
)

type DialerOptions struct {
	UnderlayDialer dialer.Dialer
	DNSClient      *Client

	FallbackDelay time.Duration
	Strategy      meta.Strategy
}
type Dialer struct {
	underlay  dialer.Dialer
	dnsClient *Client

	strategy meta.Strategy
	fallback time.Duration
}

func NewResolveDialer(options DialerOptions) *Dialer {
	if options.DNSClient == nil {
		options.DNSClient = SystemDNSClient
	}
	if options.UnderlayDialer == nil {
		options.UnderlayDialer = dialer.System
	}

	d := &Dialer{
		underlay:  options.UnderlayDialer,
		dnsClient: options.DNSClient,
	}

	return d
}

func NewResolverDialerSystem() *Dialer {
	return NewResolveDialer(DialerOptions{
		UnderlayDialer: dialer.System,
		DNSClient:      SystemDNSClient,
	})
}

func (d *Dialer) DialContext(ctx context.Context, network meta.Network, address addrs.Socksaddr) (net.Conn, error) {
	if !address.FqdnOnly() {
		return d.underlay.DialContext(ctx, network, address)
	}
	strategy := d.strategy
	dialerParallel := d.strategy != meta.StrategyIPv6Only && strategy != meta.StrategyIPv4Only &&
		network.Version == meta.NetworkVersionDual && d.fallback != 0 && network.Protocol == meta.ProtocolTCP

	if (network.Version == meta.NetworkVersion6 && strategy == meta.StrategyIPv4Only) ||
		(network.Version == meta.NetworkVersion4 && strategy == meta.StrategyIPv6Only) {
		return nil, ex.New("no available address to dial")
	}
	if network.Version == meta.NetworkVersion6 {
		strategy = meta.StrategyIPv6Only
	}
	if network.Version == meta.NetworkVersion4 {
		strategy = meta.StrategyIPv4Only
	}

	addresses, err := d.dnsClient.Lookup(ctx, address.Fqdn, strategy)
	if err != nil {
		return nil, ex.Cause(err, "lookup")
	}

	if dialerParallel {
		return dialer.DialParallel(ctx, d.underlay, network, addresses, address.Port, strategy, d.fallback)
	}

	return dialer.DialSerial(ctx, d.underlay, network, addresses, address.Port)
}

func (d *Dialer) ListenPacket(ctx context.Context, address addrs.Socksaddr) (net.PacketConn, error) {
	// Relay Service doesn't need fullcone type udp

	panic("not implemented")
}
