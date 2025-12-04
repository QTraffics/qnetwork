package dialer

import (
	"context"
	"net"
	"net/netip"
	"time"

	"github.com/qtraffics/qnetwork/meta"
	"github.com/qtraffics/qnetwork/netvars"
)

type HappyEyeballConf struct {
	FallbackDelay time.Duration
	Strategy      meta.Strategy
}

var DefaultHappyEyeballConf HappyEyeballConf = HappyEyeballConf{
	FallbackDelay: netvars.DefaultDialerFallbackDelay,
	Strategy:      meta.StrategyDefault,
}

type DefaultParallelDialer struct {
	Dialer

	Conf HappyEyeballConf
}

func (pd *DefaultParallelDialer) DialParallel(ctx context.Context, network meta.Network, address []netip.Addr, port uint16) (net.Conn, error) {
	if pd.Conf.FallbackDelay == 0 || network.Protocol == meta.ProtocolUDP || network.Version != meta.NetworkVersionDual {
		return DialSerial(ctx, pd.Dialer, network, address, port)
	}
	return DialParallel(ctx, pd.Dialer, network, address, port, pd.Conf)
}
