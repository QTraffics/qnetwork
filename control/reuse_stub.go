//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package control

func ReuseAddr() Func {
	return nil
}

func ReusePort() Func {
	return nil
}
