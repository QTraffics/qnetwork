package resolve

import (
	"context"
	"net"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/dialer"
	"github.com/qtraffics/qnetwork/meta"
	"github.com/qtraffics/qnetwork/netio"
	"github.com/qtraffics/qnetwork/netvars"
	"github.com/qtraffics/qtfra/ex"
)

type Dialer struct {
	parallelDialer dialer.ParallelDialer
	dnsClient      Client

	strategy meta.Strategy
}

func NewResolveDialer(underlay dialer.Dialer, client Client) *Dialer {
	return NewResolvDialerWithStrategy(underlay, client, meta.StrategyDefault)
}

func NewResolvDialerWithStrategy(underlay dialer.Dialer, client Client, strategy meta.Strategy) *Dialer {
	if rd, ok := underlay.(*Dialer); ok {
		rd.dnsClient = client
		rd.strategy = strategy
		return rd
	}

	rd := &Dialer{}
	if pd, ok := underlay.(dialer.ParallelDialer); ok {
		rd.parallelDialer = pd
	} else {
		rd.parallelDialer = &dialer.DefaultParallelDialer{
			Dialer: underlay,
			Conf:   dialer.HappyEyeballConf{Strategy: strategy, FallbackDelay: netvars.DefaultDialerFallbackDelay},
		}
	}

	rd.dnsClient = client
	rd.strategy = strategy
	return rd
}

func (d *Dialer) DialContext(ctx context.Context, network meta.Network, address addrs.Socksaddr) (net.Conn, error) {
	if !address.FqdnOnly() {
		return d.parallelDialer.DialContext(ctx, network, address)
	}
	strategy := d.strategy

	if (network.Version == meta.NetworkVersion6 && strategy == meta.StrategyIPv4Only) ||
		(network.Version == meta.NetworkVersion4 && strategy == meta.StrategyIPv6Only) {
		// fast filter out
		return nil, ex.New("no available address to dial")
	}

	addresses, err := d.dnsClient.Lookup(ctx, address.Fqdn, strategy)
	if err != nil {
		return nil, ex.Cause(err, "lookup")
	}
	dialerParallel := strategy != meta.StrategyIPv6Only && strategy != meta.StrategyIPv4Only &&
		network.Version == meta.NetworkVersionDual && network.Protocol == meta.ProtocolTCP && len(addresses) >= 2

	if dialerParallel {
		return d.parallelDialer.DialParallel(ctx, network, addresses, address.Port)
	}

	return dialer.DialSerial(ctx, d.parallelDialer, network, addresses, address.Port)
}

func (d *Dialer) ListenPacket(ctx context.Context, address addrs.Socksaddr) (net.PacketConn, error) {
	if !address.FqdnOnly() {
		return d.parallelDialer.ListenPacket(ctx, address)
	}
	addresses, err := d.dnsClient.Lookup(ctx, address.Fqdn, d.strategy)
	if err != nil {
		return nil, ex.Cause(err, "Lookup")
	}
	packetConn, err := netio.ListenPacketSerial(ctx, d.parallelDialer, addresses, address.Port)
	if err != nil {
		return nil, ex.Cause(err, "ListenPacketSerial")
	}
	return netio.NewBidirectionalNatConn(packetConn, address, addrs.FromNetAddr(packetConn.LocalAddr())), nil
}
