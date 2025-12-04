package resolve

import (
	"context"
	"io"
	"net/netip"
	"sync/atomic"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/meta"
	"github.com/qtraffics/qnetwork/resolve/transport"
	"github.com/qtraffics/qtfra/ex"
	"github.com/qtraffics/qtfra/threads"

	"github.com/miekg/dns"
)

type Client interface {
	Lookup(ctx context.Context, fqdn string, strategy meta.Strategy) ([]netip.Addr, error)
	Exchange(ctx context.Context, message *dns.Msg) (response *dns.Msg, err error)
}

type HeadlessClient struct {
	cache Cache

	// internal
	queryID uint32
}

func NewHeadlessClient(cache Cache) *HeadlessClient {
	c := new(HeadlessClient)
	c.queryID = uint32(dns.Id())
	c.cache = cache

	return c
}

func (c *HeadlessClient) LookupTransport(ctx context.Context, trans transport.Transport, fqdn string,
	strategy meta.Strategy,
) (addresses []netip.Addr, err error) {
	if fqdn == "" || fqdn == "." {
		return nil, ex.New("fqdn is empty")
	}
	fqdn = dns.Fqdn(fqdn)

	if strategy == meta.StrategyIPv4Only {
		return c.lookupToExchange(ctx, trans, fqdn, dns.TypeA)
	}
	if strategy == meta.StrategyIPv6Only {
		return c.lookupToExchange(ctx, trans, fqdn, dns.TypeAAAA)
	}

	var response4, response6 []netip.Addr
	var group threads.Group

	group.Append("exchange4", func(ctx context.Context) error {
		response, err := c.lookupToExchange(ctx, trans, fqdn, dns.TypeA)
		if err != nil {
			return ex.Cause(err, "lookup")
		}
		response4 = response
		return nil
	})

	group.Append("exchange6", func(ctx context.Context) error {
		response, err := c.lookupToExchange(ctx, trans, fqdn, dns.TypeAAAA)
		if err != nil {
			return ex.Cause(err, "lookup")
		}
		response6 = response
		return nil
	})

	err = group.Run(ctx)

	if len(response4) == 0 && len(response6) == 0 {
		return nil, err
	}
	return sortAddresses(response4, response6, strategy), nil
}

func (c *HeadlessClient) lookupToExchange(ctx context.Context, trans transport.Transport, fqdn string,
	typ uint16,
) (addresses []netip.Addr, err error) {
	question := dns.Question{
		Name:   fqdn,
		Qtype:  typ,
		Qclass: dns.ClassINET,
	}

	message := &dns.Msg{
		Compress: true,
		Question: []dns.Question{question},
	}
	message.Id = uint16(atomic.AddUint32(&c.queryID, 1))
	message.RecursionDesired = true

	var response *dns.Msg
	response, err = c.ExchangeTransport(ctx, trans, message)
	if err != nil || response == nil {
		return nil, err
	}
	if response.Rcode != dns.RcodeSuccess {
		return nil, NewDNSRecordError(response.Rcode)
	}
	return addrs.DNSMessageToAddresses(response), nil
}

func (c *HeadlessClient) ExchangeTransport(ctx context.Context, trans transport.Transport, message *dns.Msg) (response *dns.Msg, err error) {
	if c.cache != nil {
		response, err = c.cache.LoadOrStore(ctx, message, trans.Exchange)
		if err != nil {
			return
		}
		if response == nil {
			panic("transport returned an nil message with nil error")
		}
		return
	}
	return trans.Exchange(ctx, message)
}

func (c *HeadlessClient) Close() error {
	c.CleanCache()
	return nil
}

func (c *HeadlessClient) CleanCache() int {
	if c.cache != nil {
		return c.cache.Clear()
	}
	return 0
}

type TransportClient struct {
	*HeadlessClient

	Transport transport.Transport
}

func (c *TransportClient) Lookup(ctx context.Context, fqdn string, strategy meta.Strategy) (addresses []netip.Addr, err error) {
	return c.HeadlessClient.LookupTransport(ctx, c.Transport, fqdn, strategy)
}

func (c *TransportClient) Exchange(ctx context.Context, message *dns.Msg) (response *dns.Msg, err error) {
	return c.HeadlessClient.ExchangeTransport(ctx, c.Transport, message)
}

func (c *TransportClient) Close() error {
	var err error
	if cc, ok := c.Transport.(io.Closer); ok {
		err = cc.Close()
	}
	return ex.Errors(err, c.HeadlessClient.Close())
}
