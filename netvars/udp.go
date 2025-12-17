package netvars

import "time"

const (
	DefaultUDPReadBufferSize = 65507
	DefaultUDPKeepAlive      = 60 * time.Second
	DefaultUDPConnSize       = 1024
)
