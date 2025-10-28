package dialer

import (
	"context"
	"net"
	"net/netip"
	"time"

	"github.com/QTraffics/qnetwork"
	"github.com/QTraffics/qnetwork/addrs"
	"github.com/QTraffics/qnetwork/meta"
	"github.com/QTraffics/qtfra/enhancements/slicelib"
	"github.com/QTraffics/qtfra/ex"
)

func DialParallel(ctx context.Context, dialer Dialer, network meta.Network, addresses []netip.Addr, port uint16, strategy meta.Strategy, fallbackDelay time.Duration) (net.Conn, error) {
	preferIPv6 := strategy == meta.StrategyPreferIPv6
	if fallbackDelay == 0 {
		fallbackDelay = qnetwork.DefaultDialerFallbackDelay
	}

	returned := make(chan struct{})
	defer close(returned)

	addresses4 := slicelib.Filter(addresses, addrs.Is4)
	addresses6 := slicelib.Filter(addresses, addrs.Is6)

	if len(addresses4) == 0 || len(addresses6) == 0 {
		return DialSerial(ctx, dialer, network, addresses, port)
	}
	if network.Version == meta.NetworkVersion4 || strategy == meta.StrategyIPv4Only {
		return DialSerial(ctx, dialer, network, addresses4, port)
	}

	if network.Version == meta.NetworkVersion6 || strategy == meta.StrategyIPv6Only {
		return DialSerial(ctx, dialer, network, addresses6, port)
	}

	var primaries, fallbacks []netip.Addr
	if preferIPv6 {
		primaries = addresses6
		fallbacks = addresses4
	} else {
		primaries = addresses4
		fallbacks = addresses6
	}
	type dialResult struct {
		net.Conn
		error
		primary bool
		done    bool
	}
	results := make(chan dialResult)
	startRacer := func(ctx context.Context, primary bool) {
		ras := primaries
		if !primary {
			ras = fallbacks
		}
		c, err := DialSerial(ctx, dialer, network, ras, port)
		select {
		case results <- dialResult{Conn: c, error: err, primary: primary, done: true}:
		case <-returned:
			if c != nil {
				c.Close()
			}
		}
	}
	var primary, fallback dialResult
	primaryCtx, primaryCancel := context.WithCancel(ctx)
	defer primaryCancel()
	go startRacer(primaryCtx, true)
	fallbackTimer := time.NewTimer(fallbackDelay)
	defer fallbackTimer.Stop()
	for {
		select {
		case <-fallbackTimer.C:
			fallbackCtx, fallbackCancel := context.WithCancel(ctx)
			defer fallbackCancel()
			go func() {
				startRacer(fallbackCtx, false)
			}()

		case res := <-results:
			if res.error == nil {
				return res.Conn, nil
			}
			if res.primary {
				primary = res
			} else {
				fallback = res
			}
			if primary.done && fallback.done {
				return nil, primary.error
			}
			if res.primary && fallbackTimer.Stop() {
				fallbackTimer.Reset(0)
			}
		}
	}
}

func DialSerial(ctx context.Context, this Dialer, network meta.Network, address []netip.Addr, port uint16) (net.Conn, error) {
	if len(address) == 0 {
		return nil, ex.New("no address to dial")
	}
	var errs error
	for _, aa := range address {
		if !aa.IsValid() {
			errs = ex.Errors(errs, ex.New("invalid address"))
			continue
		}
		internalConn, internalErr := this.DialContext(ctx, network,
			addrs.FromAddrPort(netip.AddrPortFrom(aa, port)))
		if internalErr == nil {
			return internalConn, nil
		}
		errs = ex.Errors(errs, internalErr)
	}
	return nil, ex.New("DialSerial all addresses failed :", errs)
}
