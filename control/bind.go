package control

import (
	"net/netip"
	"syscall"

	"github.com/QTraffics/qnetwork/addrs"
	"github.com/QTraffics/qtfra/ex"
)

func BindToInterface(finder InterfaceFinder, interfaceName string, interfaceIndex int) Func {
	return func(network, address string, conn syscall.RawConn) error {
		return BindToInterface0(finder, conn, network, address, interfaceName, interfaceIndex, false)
	}
}

func BindToInterfaceFunc(finder InterfaceFinder, block func(network string, address string) (interfaceName string, interfaceIndex int, err error)) Func {
	return func(network, address string, conn syscall.RawConn) error {
		interfaceName, interfaceIndex, err := block(network, address)
		if err != nil {
			return err
		}
		return BindToInterface0(finder, conn, network, address, interfaceName, interfaceIndex, false)
	}
}

func BindToInterface0(finder InterfaceFinder, conn syscall.RawConn, network string, address string, interfaceName string, interfaceIndex int, preferInterfaceName bool) error {
	if interfaceName == "" && interfaceIndex == -1 {
		return ex.New("interface not found: ", interfaceName)
	}
	if addr := addrs.FromParseSocksaddr(address).Addr; addr.IsValid() && isVirtualInterface(addr) {
		return nil
	}
	return bindToInterface(conn, network, address, finder, interfaceName, interfaceIndex, preferInterfaceName)
}

func isVirtualInterface(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsMulticast() || addr.IsInterfaceLocalMulticast()
}
