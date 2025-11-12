package resolve

import (
	"context"
	"math"
	"time"

	"github.com/qtraffics/qnetwork/netvars"
	"github.com/qtraffics/qtfra/enhancements/singleflight"
	"github.com/qtraffics/qtfra/ex"

	"github.com/elastic/go-freelru"
	"github.com/miekg/dns"
)

type Cache interface {
	LoadOrStore(ctx context.Context, message *dns.Msg,
		constructor func(ctx context.Context, message *dns.Msg) (*dns.Msg, error)) (*dns.Msg, error)
	Store(message *dns.Msg) bool
	Clear() int
}

type CacheEntry struct {
	Expire  time.Time
	Message *dns.Msg
}

type defaultCache struct {
	lru            freelru.Cache[dns.Question, CacheEntry]
	minTTL, maxTTL uint32

	sf singleflight.Group[uint32, *dns.Msg]
}

func (c *defaultCache) LoadOrStore(ctx context.Context, message *dns.Msg, constructor func(ctx context.Context, message *dns.Msg) (*dns.Msg, error)) (*dns.Msg, error) {
	if len(message.Question) == 0 {
		response := &dns.Msg{
			MsgHdr: dns.MsgHdr{
				Id:       message.Id,
				Response: true,
				Rcode:    dns.RcodeFormatError,
			},
			Question: message.Question,
		}

		return response, nil
	}

	if !c.enableCache(message) {
		if constructor != nil {
			return constructor(ctx, message)
		}
		return nil, nil
	}
	messageId := message.Id
	question := message.Question[0]

	// check cache is valid.
	if answer, cached := c.lru.Get(question); cached {
		beforeConstructor := time.Now()
		if answer.Expire.IsZero() || answer.Expire.Before(beforeConstructor) || answer.Message == nil {
			cached = false
		}

		ttl := uint32(answer.Expire.Sub(beforeConstructor) / time.Second)
		if ttl == 0 {
			cached = false
		}

		if cached {
			response := answer.Message.Copy()
			response.Id = messageId
			OverwriteTTL(response, ttl)

			return EdnsBackwards(message, response), nil
		}
		c.lru.Remove(question)
	}
	if constructor == nil {
		return nil, nil
	}

	response, err, _ := c.sf.Do(hashQuestion(question), func() (*dns.Msg, error) {
		response, err := constructor(ctx, message)
		if err != nil || response == nil {
			return nil, err
		}
		if response.Rcode != dns.RcodeSuccess {
			return response, nil
		}
		c.Store(response)
		return response, nil
	})
	if err != nil {
		return nil, err
	}
	response = response.Copy()
	response = EdnsBackwards(message, response)
	response.Id = messageId
	return response, nil
}

func (c *defaultCache) Store(message *dns.Msg) bool {
	if len(message.Question) != 1 {
		return false
	}

	ttl := CalculateTTL(message)
	ttl = c.overwriteTTL(ttl)
	if ttl <= 1 {
		return false
	}
	question := message.Question[0]
	entry := CacheEntry{
		Expire:  time.Now().Add(time.Duration(ttl)),
		Message: message,
	}
	c.lru.Add(question, entry)
	return true
}

func (c *defaultCache) overwriteTTL(ttl uint32) uint32 {
	ttl = max(c.minTTL, ttl)
	ttl = min(c.maxTTL, ttl)
	return ttl
}

func (c *defaultCache) enableCache(message *dns.Msg) bool {
	return message != nil && len(message.Question) == 1
}

func (c *defaultCache) Clear() int {
	cc := c.lru.Len()
	c.lru.Purge()
	return cc
}

type CacheOptions struct {
	Size   uint32
	MaxTTL uint32
	MinTTL uint32
}

func NewCacheSize(opt CacheOptions) (Cache, error) {
	if opt.Size <= 0 {
		return nil, ex.New("negative cache size")
	}
	if opt.MaxTTL < opt.MinTTL {
		opt.MaxTTL = opt.MinTTL
	}

	c := &defaultCache{}
	c.lru = ex.Must0(freelru.NewSharded[dns.Question, CacheEntry](opt.Size, hashQuestion))
	c.minTTL, c.maxTTL = opt.MinTTL, opt.MaxTTL

	return c, nil
}

func NewCache() Cache {
	return ex.Must0(NewCacheSize(
		CacheOptions{
			Size:   netvars.DefaultResolverCacheSize,
			MinTTL: 0,
			MaxTTL: math.MaxUint32,
		}))
}
