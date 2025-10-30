package addrs

import (
	"net"
	"net/netip"
	"strconv"
)

type Socksaddr struct {
	Fqdn string
	Addr netip.Addr
	Port uint16
}

func FromParseSocksaddr(address string) Socksaddr {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return FromParseSocksaddrHostPort(address, 0)
	}
	return FromParseSocksaddrHostPortStr(host, port)
}

func FromParseSocksaddrHostPort(host string, port uint16) Socksaddr {
	netAddr, err := netip.ParseAddr(UnwrapIPv6Address(host))
	if err != nil {
		return Socksaddr{
			Fqdn: host,
			Port: port,
		}
	}

	return Socksaddr{
		Addr: netAddr,
		Port: port,
	}
}

func FromParseSocksaddrHostPortStr(host string, portStr string) Socksaddr {
	port, _ := strconv.Atoi(portStr)
	netAddr, err := netip.ParseAddr(UnwrapIPv6Address(host))
	if err != nil {
		return Socksaddr{
			Fqdn: host,
			Port: uint16(port),
		}
	}

	return Socksaddr{
		Addr: netAddr,
		Port: uint16(port),
	}
}

func FromNetAddr(netAddr net.Addr) Socksaddr {
	ap := AddrPortFromNetAddr(netAddr)
	return FromAddrPort(ap)
}

func FromAddrPort(ap netip.AddrPort) Socksaddr {
	return Socksaddr{
		Addr: ap.Addr(),
		Port: ap.Port(),
	}
}

func (a Socksaddr) FqdnOnly() bool {
	return !a.Addr.IsValid()
}

func (a Socksaddr) Network() string {
	return "socks"
}

func (a Socksaddr) String() string {
	if a.Addr.IsValid() {
		return netip.AddrPortFrom(a.Addr, a.Port).String()
	}
	return net.JoinHostPort(a.AddrString(), strconv.FormatUint(uint64(a.Port), 10))
}

func (a Socksaddr) Dialable() bool {
	return (a.Addr.IsValid() || a.Fqdn != "") && a.Port != 0
}

func (a Socksaddr) NeedResolve() bool {
	return !a.Addr.IsValid() && a.Fqdn != ""
}

func (a Socksaddr) Unwrap() Socksaddr {
	if a.Addr.Is4In6() {
		return Socksaddr{
			Addr: netip.AddrFrom4(a.Addr.As4()),
			Port: a.Port,
		}
	}
	return a
}

func (a Socksaddr) AddrString() string {
	if a.Addr.IsValid() {
		return a.Addr.String()
	}
	return a.Fqdn
}

func (a Socksaddr) TCPAddr() *net.TCPAddr {
	return &net.TCPAddr{
		IP:   a.Addr.AsSlice(),
		Port: int(a.Port),
		Zone: a.Addr.Zone(),
	}
}

func (a Socksaddr) UDPAddr() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   a.Addr.AsSlice(),
		Port: int(a.Port),
		Zone: a.Addr.Zone(),
	}
}

func (a Socksaddr) IPAddr() *net.IPAddr {
	return &net.IPAddr{
		IP:   a.Addr.AsSlice(),
		Zone: a.Addr.Zone(),
	}
}

func (a Socksaddr) AddrPort() netip.AddrPort {
	return netip.AddrPortFrom(a.Addr, a.Port)
}
