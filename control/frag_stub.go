//go:build !linux

package control

func DisableUDPFragment() Func {
	return nil
}
