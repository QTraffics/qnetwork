package addrs

import (
	"net"
	"net/netip"
	urlpkg "net/url"
	"strings"

	"github.com/QTraffics/qtfra/enhancements/slicelib"
	"github.com/miekg/dns"
)

func AddrFromIP(ip net.IP) netip.Addr {
	addr, _ := netip.AddrFromSlice(ip)
	return addr
}

func AddrPortFromNetAddr(netAddr net.Addr) netip.AddrPort {
	var ip net.IP
	var port uint16
	switch addr := netAddr.(type) {
	case Socksaddr:
		return addr.AddrPort()
	case *net.TCPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.UDPAddr:
		ip = addr.IP
		port = uint16(addr.Port)
	case *net.IPAddr:
		ip = addr.IP
	}
	return netip.AddrPortFrom(AddrFromIP(ip), port)
}

func UnwrapIPv6Address(address string) string {
	if len(address) > 2 && address[0] == '[' && address[len(address)-1] == ']' {
		return address[1 : len(address)-1]
	}
	return address
}

func PrefixFromNet(netAddr net.Addr) netip.Prefix {
	switch addr := netAddr.(type) {
	case *net.IPNet:
		bits, _ := addr.Mask.Size()
		return netip.PrefixFrom(AddrFromIP(addr.IP).Unmap(), bits)
	default:
		return netip.Prefix{}
	}
}

func DNSMessageToAddresses(response *dns.Msg) []netip.Addr {
	addresses := make([]netip.Addr, 0, len(response.Answer))
	for _, rawAnswer := range response.Answer {
		switch answer := rawAnswer.(type) {
		case *dns.A:
			addresses = append(addresses, AddrFromIP(answer.A))
		case *dns.AAAA:
			addresses = append(addresses, AddrFromIP(answer.AAAA))
		case *dns.HTTPS:
			for _, value := range answer.SVCB.Value {
				if value.Key() == dns.SVCB_IPV4HINT || value.Key() == dns.SVCB_IPV6HINT {
					addresses = append(addresses, slicelib.Map[string, netip.Addr](strings.Split(value.String(), ","), func(it string) netip.Addr {
						a, _ := netip.ParseAddr(it)
						return a
					})...)
				}
			}
		}
	}
	return addresses
}

func FqdnToDomain(fqdn string) string {
	if dns.IsFqdn(fqdn) {
		return fqdn[:len(fqdn)-1]
	}
	return fqdn
}

func Is4(ip netip.Addr) bool {
	return ip.Is4() || ip.Is4In6()
}
func Is6(ip netip.Addr) bool {
	return ip.Is6() && !ip.Is4In6()
}

func CopyURL(raw *urlpkg.URL) *urlpkg.URL {
	return &urlpkg.URL{
		Scheme:      raw.Scheme,
		Opaque:      raw.Opaque,
		User:        copyUrlUser(raw.User),
		Host:        raw.Host,
		Path:        raw.Path,
		RawPath:     raw.RawPath,
		OmitHost:    raw.OmitHost,
		ForceQuery:  raw.ForceQuery,
		RawQuery:    raw.RawQuery,
		Fragment:    raw.Fragment,
		RawFragment: raw.RawFragment,
	}

}

func copyUrlUser(u *urlpkg.Userinfo) *urlpkg.Userinfo {
	if u == nil {
		return nil
	}

	pp, ps := u.Password()
	if ps {
		return urlpkg.UserPassword(u.Username(), pp)
	}
	return urlpkg.User(u.Username())
}
