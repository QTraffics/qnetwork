package netvars

import "time"

const (
	DefaultTCPKeepAliveInitial    = 10 * time.Minute
	DefaultTCPKeepAliveInterval   = 75 * time.Second
	DefaultTCPKeepAliveProbeCount = 16

	DefaultUDPReadBufferSize = 65507
	DefaultUDPKeepAlive      = 60 * time.Second

	DefaultDialerFallbackDelay = 300 * time.Millisecond
	DefaultDialerTimeout       = 5 * time.Second

	DefaultResolverReadTimeout = 5 * time.Second
	DefaultResolverTTL         = 600 // seconds
	DefaultResolverCacheSize   = 1024
)

const (
	MaxDNSUDPSize = 1232
)
