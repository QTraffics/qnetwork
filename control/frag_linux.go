package control

import (
	"os"
	"syscall"

	"github.com/qtraffics/qnetwork/meta"

	"golang.org/x/sys/unix"
)

func DisableUDPFragment() Func {
	return func(network, address string, conn syscall.RawConn) error {
		var mn meta.Network
		var ok bool
		if mn, ok = meta.ParseNetwork(network); !ok || mn.Protocol != meta.ProtocolUDP {
			return nil
		}
		return Raw(conn, func(fd uintptr) error {
			if mn.Version == meta.NetworkVersion4 || mn.Version == meta.NetworkVersionDual {
				err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO)
				if err != nil {
					return os.NewSyscallError("SETSOCKOPT IP_MTU_DISCOVER IP_PMTUDISC_DO", err)
				}
			}
			if mn.Version == meta.NetworkVersion6 || mn.Version == meta.NetworkVersionDual {
				err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_MTU_DISCOVER, unix.IP_PMTUDISC_DO)
				if err != nil {
					return os.NewSyscallError("SETSOCKOPT IPV6_MTU_DISCOVER IP_PMTUDISC_DO", err)
				}
			}
			return nil
		})
	}
}
