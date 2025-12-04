package addrs

import (
	"net/netip"
	"slices"
	"sort"

	"github.com/qtraffics/qnetwork/meta"
)

func SortAddresses(addresses []netip.Addr, strategy meta.Strategy) []netip.Addr {
	sorted := slices.Clone(addresses)
	SortAddressesInPlace(sorted, strategy)
	return sorted
}

func SortAddressesInPlace(addresses []netip.Addr, strategy meta.Strategy) {
	if strategy == meta.StrategyIPv4Only || strategy == meta.StrategyIPv6Only || len(addresses) <= 1 {
		return
	}

	preferIPv4 := strategy == meta.StrategyPreferIPv4

	if !preferIPv4 && strategy != meta.StrategyPreferIPv6 {
		return
	}

	sort.Slice(addresses, func(i, j int) bool {
		a4 := addresses[i].Is4()
		b4 := addresses[j].Is4()

		if preferIPv4 {
			return a4 && !b4
		}
		return !a4 && b4
	})
}

func FilterAddressByStrategy(addresses []netip.Addr, strategy meta.Strategy) []netip.Addr {
	var filtered []netip.Addr
	//nolint:exhaustive
	switch strategy {
	case meta.StrategyIPv4Only:
		for _, addr := range addresses {
			if addr.Is4() {
				filtered = append(filtered, addr)
			}
		}
	case meta.StrategyIPv6Only:
		for _, addr := range addresses {
			if addr.Is6() {
				filtered = append(filtered, addr)
			}
		}
	default:
		return addresses
	}
	return filtered
}
