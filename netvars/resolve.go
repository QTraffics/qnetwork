package netvars

import "time"

const (
	DefaultResolverReadTimeout = 5 * time.Second
	DefaultResolverTTL         = 600 // seconds
	DefaultResolverCacheSize   = 1024

	MaxDNSUDPSize = 1232
)
