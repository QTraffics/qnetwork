package grpclib

import (
	"context"
	"math/rand/v2"
	"time"

	"google.golang.org/grpc/backoff"
)

func Backoff(config backoff.Config, retries uint64) time.Duration {
	if retries <= 0 {
		return config.BaseDelay
	}
	bf, max := float64(config.BaseDelay), float64(config.MaxDelay)
	for bf < max && retries > 0 {
		bf *= config.Multiplier
		retries--
	}
	if bf > max {
		bf = max
	}
	// Randomize backoff delays so that if a cluster of requests start at
	// the same time, they won't operate in lockstep.
	bf *= 1 + config.Jitter*(rand.Float64()*2-1)
	if bf < 0 {
		return 0
	}
	return time.Duration(bf)
}

type BackOffTicker struct {
	backoff backoff.Config
	count   uint64
}

func NewBackoffTicker(bf backoff.Config, initialCount uint64) *BackOffTicker {
	return &BackOffTicker{backoff: bf, count: initialCount}
}

func NewBackoffTicker0(bf backoff.Config) *BackOffTicker {
	return &BackOffTicker{backoff: bf, count: 0}
}

func (bt *BackOffTicker) Wait(ctx context.Context) bool {
	bt.count++
	select {
	case <-ctx.Done():
		return false
	case <-time.After(Backoff(bt.backoff, bt.count)):
		return true
	}
}

func (bt *BackOffTicker) Reset() {
	bt.count = 0
}
