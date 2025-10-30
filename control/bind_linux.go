package control

import (
	"os"
	"sync/atomic"
	"syscall"

	"github.com/qtraffics/qtfra/ex"

	"golang.org/x/sys/unix"
)

var ifIndexDisabled atomic.Bool

func bindToInterface(conn syscall.RawConn, network string, address string, finder InterfaceFinder, interfaceName string, interfaceIndex int, preferInterfaceName bool) error {
	return Raw(conn, func(fd uintptr) error {
		if preferInterfaceName || ifIndexDisabled.Load() {
			if interfaceName == "" {
				return os.ErrInvalid
			}
			return unix.BindToDevice(int(fd), interfaceName)
		}

		// Attempt to use interface index
		idx := interfaceIndex
		if idx == -1 {
			if interfaceName == "" {
				return os.ErrInvalid
			}
			iif, err := finder.ByName(interfaceName)
			if err != nil {
				return err
			}
			idx = iif.Index
		}

		err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BINDTOIFINDEX, idx)
		if err == nil {
			return nil
		}

		if ex.IsMulti(err, unix.ENOPROTOOPT, unix.EINVAL) {
			ifIndexDisabled.Store(true)
		} else {
			return err
		}

		// Fallback to binding by name
		if interfaceName == "" {
			return os.ErrInvalid
		}
		return unix.BindToDevice(int(fd), interfaceName)
	})
}
