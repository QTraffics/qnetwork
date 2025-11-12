package addrs

import (
	"net/netip"
	"sort"

	"github.com/qtraffics/qnetwork/meta"
)

func SortAddresses(addresses []netip.Addr, strategy meta.Strategy) []netip.Addr {
	if len(addresses) == 0 {
		return nil
	}
	sorted := make([]netip.Addr, len(addresses))
	copy(sorted, addresses)
	var ipv4First bool
	switch strategy {
	case meta.StrategyPreferIPv4, meta.StrategyIPv4Only:
		ipv4First = true
	case meta.StrategyPreferIPv6, meta.StrategyIPv6Only, meta.StrategyDefault:
		ipv4First = false
	default:
		return sorted
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if ipv4First {
			if a.Is4() && !b.Is4() {
				return true
			}
			if !a.Is4() && b.Is4() {
				return false
			}
		} else {
			if a.Is6() && !b.Is6() {
				return true
			}
			if !a.Is6() && b.Is6() {
				return false
			}
		}
		return false
	})
	return sorted
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
