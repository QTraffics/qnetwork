package netvars

import "time"

const (
	DefaultTCPKeepAliveInitial    = 5 * time.Minute
	DefaultTCPKeepAliveInterval   = 75 * time.Second
	DefaultTCPKeepAliveProbeCount = 16

	DefaultDialerFallbackDelay = 300 * time.Millisecond
	DefaultDialerTimeout       = 5 * time.Second
)
