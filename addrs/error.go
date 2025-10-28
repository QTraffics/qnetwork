package addrs

import "github.com/QTraffics/qtfra/ex"

var (
	ErrNotDialable        = ex.New("address can not used to dial a tcp or udp network")
	ErrAddressNotResolved = ex.New("address not resolved")
)
