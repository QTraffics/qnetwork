package resolve

import (
	"context"
	"io"
	"net/netip"

	"github.com/QTraffics/qnetwork"
	"github.com/QTraffics/qnetwork/addrs"
	"github.com/QTraffics/qnetwork/dialer"
	"github.com/QTraffics/qnetwork/meta"
	"github.com/QTraffics/qnetwork/resolve/hosts"
	"github.com/QTraffics/qnetwork/resolve/transport"
	"github.com/QTraffics/qtfra/ex"
	"github.com/QTraffics/qtfra/threads"
	"github.com/miekg/dns"
)

var SystemDNSClient = NewClient(ClientOptions{
	WithTransport: transport.NewLocalTransport(context.Background(), transport.LocalTransportOptions{
		Dialer: dialer.System,
		Host:   hosts.NewFileDefault(),
	}),
})

type ClientOptions struct {
	DisableCache  bool
	WithCache     Cache
	WithTransport transport.Transport
}

type Client struct {
	ctx context.Context

	transport transport.Transport
	cache     Cache

	disableCache bool
}

func (c *Client) Lookup(ctx context.Context, fqdn string, strategy meta.Strategy) (addresses []netip.Addr, err error) {
	if fqdn == "" || fqdn == "." {
		return nil, ex.New("fqdn is empty")
	}
	fqdn = dns.Fqdn(fqdn)

	if strategy == meta.StrategyIPv4Only {
		return c.lookupToExchange(ctx, fqdn, dns.TypeA)
	}
	if strategy == meta.StrategyIPv6Only {
		return c.lookupToExchange(ctx, fqdn, dns.TypeAAAA)
	}

	var response4, response6 []netip.Addr
	var group threads.Group

	group.Append("exchange4", func(ctx context.Context) error {
		response, err := c.lookupToExchange(ctx, fqdn, dns.TypeA)
		if err != nil {
			return ex.Cause(err, "lookup")
		}
		response4 = response
		return nil
	})

	group.Append("exchange6", func(ctx context.Context) error {
		response, err := c.lookupToExchange(ctx, fqdn, dns.TypeAAAA)
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

func (c *Client) lookupToExchange(ctx context.Context, fqdn string, qtype uint16) (addresses []netip.Addr, err error) {
	question := dns.Question{
		Name:   fqdn,
		Qtype:  qtype,
		Qclass: dns.ClassINET,
	}

	requestEdns0 := &dns.OPT{}
	requestEdns0.SetUDPSize(qnetwork.MaxDNSUDPSize)

	message := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:               dns.Id(),
			RecursionDesired: true,
		},
		Compress: true,
		Question: []dns.Question{question},
		Extra:    []dns.RR{requestEdns0},
	}

	var response *dns.Msg
	response, err = c.Exchange(ctx, message)
	if err != nil || response == nil {
		return nil, err
	}
	if response.Rcode != dns.RcodeSuccess {
		return nil, NewDNSRecordError(response.Rcode)
	}
	return addrs.DNSMessageToAddresses(response), nil
}

func (c *Client) Exchange(ctx context.Context, message *dns.Msg) (response *dns.Msg, err error) {
	if !c.disableCache && c.cache != nil {
		response, err = c.cache.LoadOrStore(ctx, message, c.transport.Exchange)
		if err != nil {
			return
		}
		if response == nil {
			return nil, ex.New("transport returned an nil message with nil error")
		}
		return
	}
	return c.transport.Exchange(ctx, message)
}

func (c *Client) Close() error {
	if c.cache != nil {
		c.cache.Clear()
	}
	var err error
	if cc, ok := c.transport.(io.Closer); ok {
		err = cc.Close()
	}
	return err
}

func (c *Client) ClearCache() int {
	var nn int
	if c.cache != nil {
		nn = c.cache.Clear()
	}
	return nn
}

func NewClient(option ClientOptions) *Client {
	return NewClientContext(context.Background(), option)
}

func NewClientContext(ctx context.Context, option ClientOptions) *Client {
	if option.WithTransport == nil {
		option.WithTransport = transport.NewLocalTransport(ctx, transport.LocalTransportOptions{
			Dialer: dialer.System,
			Host:   hosts.NewFileDefault(),
		})
	}

	if !option.DisableCache && option.WithCache == nil {
		option.WithCache = NewCache()
	}

	c := &Client{
		ctx:          ctx,
		transport:    option.WithTransport,
		cache:        option.WithCache,
		disableCache: option.DisableCache,
	}

	return c
}

func sortAddresses(addresses4 []netip.Addr, addresses6 []netip.Addr, strategy meta.Strategy) []netip.Addr {
	if strategy == meta.StrategyPreferIPv6 || strategy == meta.StrategyDefault {
		return append(addresses6, addresses4...)
	} else {
		return append(addresses4, addresses6...)
	}
}
