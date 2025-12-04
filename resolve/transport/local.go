package transport

import (
	"bufio"
	"context"
	"net"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/dialer"
	"github.com/qtraffics/qnetwork/netvars"
	"github.com/qtraffics/qnetwork/resolve/hosts"
	"github.com/qtraffics/qtfra/enhancements/maplib"
	"github.com/qtraffics/qtfra/ex"

	"github.com/miekg/dns"
)

var _ Transport = (*LocalTransport)(nil)

type LocalTransportOptions struct {
	Dialer dialer.Dialer
	Host   *hosts.File
}

type LocalTransport struct {
	// options
	host   *hosts.File
	dialer dialer.Dialer

	// cache
	transportAccess sync.RWMutex
	transports      map[string]*UDPTransport
	servers         maplib.Set[string]
}

func NewLocalTransport(options *LocalTransportOptions) *LocalTransport {
	if options == nil {
		options = &LocalTransportOptions{
			Dialer: dialer.System,
			Host:   hosts.NewFileDefault(),
		}
	}

	if options.Dialer == nil {
		options.Dialer = dialer.System
	}
	return &LocalTransport{
		host:   options.Host,
		dialer: options.Dialer,
	}
}

func (t *LocalTransport) Exchange(ctx context.Context, message *dns.Msg) (*dns.Msg, error) {
	if len(message.Question) == 0 {
		return nil, ex.New("bad message: no question found")
	}
	question := message.Question[0]
	domain := addrs.FqdnToDomain(question.Name)
	if (question.Qtype == dns.TypeA || question.Qtype == dns.TypeAAAA) && t.host != nil {
		address := t.host.Lookup(domain)
		if len(address) > 0 {
			return FixedResponse(message.Id, question, address, netvars.DefaultResolverTTL), nil
		}
	}
	systemConfig := getSystemDNSConfig(ctx)
	if systemConfig.singleRequest || !(message.Question[0].Qtype == dns.TypeA || message.Question[0].Qtype == dns.TypeAAAA) {
		return t.exchangeSingleRequest(ctx, systemConfig, message, domain)
	}
	return t.exchangeParallel(ctx, systemConfig, message, domain)
}

func (t *LocalTransport) exchangeSingleRequest(ctx context.Context, systemConfig *dnsConfig, message *dns.Msg, domain string) (*dns.Msg, error) {
	var lastErr error
	for _, fqdn := range systemConfig.nameList(domain) {
		response, err := t.tryOneName(ctx, systemConfig, fqdn, message)
		if err != nil {
			lastErr = err
			continue
		}
		return response, nil
	}
	return nil, lastErr
}

func (t *LocalTransport) exchangeParallel(ctx context.Context, systemConfig *dnsConfig, message *dns.Msg, domain string) (*dns.Msg, error) {
	returned := make(chan struct{})
	defer close(returned)
	type queryResult struct {
		response *dns.Msg
		err      error
	}
	results := make(chan queryResult)
	startRacer := func(ctx context.Context, fqdn string) {
		response, err := t.tryOneName(ctx, systemConfig, fqdn, message)
		select {
		case results <- queryResult{response, err}:
		case <-returned:
		}
	}
	queryCtx, queryCancel := context.WithCancel(ctx)
	defer queryCancel()
	var nameCount int
	for _, fqdn := range systemConfig.nameList(domain) {
		nameCount++
		go startRacer(queryCtx, fqdn)
	}
	var errors []error
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-results:
			if result.err == nil {
				return result.response, nil
			}
			errors = append(errors, result.err)
			if len(errors) == nameCount {
				return nil, ex.Errors(errors...)
			}
		}
	}
}

func (t *LocalTransport) tryOneName(ctx context.Context, config *dnsConfig, fqdn string, message *dns.Msg) (*dns.Msg, error) {
	serverOffset := config.serverOffset()
	sLen := uint32(len(config.servers))
	t.updateTransports(config.servers)
	messageId := message.Id
	if messageId == 0 {
		messageId = dns.Id()
	}
	var lastErr error
	for i := 0; i < config.attempts; i++ {
		for j := uint32(0); j < sLen; j++ {
			server := config.servers[(serverOffset+j)%sLen]
			question := message.Question[0]
			question.Name = dns.Fqdn(fqdn)
			addr := addrs.FromParseSocksaddr(server)

			// Must set the port to 53 here.
			// Due to the pullTransport does not match the server port.
			addr.Port = 53

			transport, err := t.pullTransport(addr)
			if err != nil {
				return nil, ex.Cause(err, "pull transport")
			}
			response, err := t.exchangeOne(ctx,
				transport,
				messageId,
				question,
				config.timeout,
				config.useTCP,
				config.trustAD)
			if err != nil {
				lastErr = err
				continue
			}
			return response, nil
		}
	}
	return nil, ex.Cause(lastErr, fqdn)
}

func (t *LocalTransport) exchangeOne(ctx context.Context, transport *UDPTransport, messageID uint16,
	question dns.Question, timeout time.Duration, useTCP, ad bool,
) (*dns.Msg, error) {
	if timeout == 0 {
		timeout = netvars.DefaultResolverReadTimeout
	}
	if transport == nil {
		return nil, ex.New("missing transport")
	}
	exMessage := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:                messageID,
			RecursionDesired:  true,
			AuthenticatedData: ad,
		},
		Compress: true,
		Question: []dns.Question{question},
	}
	exMessage.SetEdns0(maxUDPSize, false)

	if useTCP {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return transport.tcp.Exchange(timeoutCtx, exMessage)
	}
	udpCtx, udpCancel := context.WithTimeout(ctx, timeout)
	defer udpCancel()
	response, needTCP, err := transport.exchange(udpCtx, exMessage)
	if err != nil && !needTCP {
		return nil, ex.Cause(err, "exchange udp")
	}

	if needTCP || response.Truncated {
		tcpCtx, tcpCancel := context.WithTimeout(ctx, timeout)
		defer tcpCancel()
		response, err = transport.tcp.Exchange(tcpCtx, exMessage)
		if err != nil {
			return nil, ex.Cause(err, "exchange tcp")
		}
	}
	return response, nil
}

func (t *LocalTransport) updateTransports(servers []string) {
	t.transportAccess.Lock()
	defer t.transportAccess.Unlock()
	if t.transports == nil {
		t.transports = make(map[string]*UDPTransport)
	}

	if t.servers != nil && len(t.servers) == len(servers) && t.servers.ContainAll(servers) {
		return
	}
	if len(servers) == 0 {
		// close all
		t.servers = maplib.NewSet[string]()
		for dd, transport := range t.transports {
			_ = transport.Close()
			delete(t.transports, dd)
		}
		return
	}

	t.servers = maplib.NewSetFromSlice(servers)
	for addr, tt := range t.transports {
		if !t.servers.Contains(addr) {
			_ = tt.Close()
			delete(t.transports, addr)
		}
	}

	// create new udp transport at pullTransport.
}

func (t *LocalTransport) pullTransport(server addrs.Socksaddr) (*UDPTransport, error) {
	addressStr := server.AddrString()
	t.transportAccess.RLock()
	if t.servers == nil || !t.servers.Contains(addressStr) {
		t.transportAccess.RUnlock()
		return nil, ex.New("dns server not configured, may resolve file changed: ", addressStr)
	}
	if tt, found := t.transports[addressStr]; found {
		t.transportAccess.RUnlock()
		return tt, nil
	}
	t.transportAccess.RUnlock()

	t.transportAccess.Lock()
	defer t.transportAccess.Unlock()
	if t.servers == nil || !t.servers.Contains(addressStr) {
		return nil, ex.New("dns server not configured, may resolve file changed: ", addressStr)
	}
	// find again
	if tt, found := t.transports[addressStr]; found {
		return tt, nil
	}

	tt := NewUDP(server, UDPTransportOptions{Dialer: t.dialer})

	t.transports[addressStr] = tt

	return tt, nil
}

func dnsReadConfig(_ context.Context, name string) *dnsConfig {
	conf := &dnsConfig{
		ndots:    1,
		timeout:  5 * time.Second,
		attempts: 2,
	}
	file, err := os.Open(name)
	if err != nil {
		conf.servers = defaultNS
		conf.search = dnsDefaultSearch()
		conf.err = err
		return conf
	}
	defer file.Close()
	fi, err := file.Stat()
	if err == nil {
		conf.mtime = fi.ModTime()
	} else {
		conf.servers = defaultNS
		conf.search = dnsDefaultSearch()
		conf.err = err
		return conf
	}
	reader := bufio.NewReader(file)
	var (
		prefix   []byte
		line     []byte
		isPrefix bool
	)
	for {
		line, isPrefix, err = reader.ReadLine()
		if err != nil {
			break
		}
		if isPrefix {
			prefix = append(prefix, line...)
			continue
		} else if len(prefix) > 0 {
			line = append(prefix, line...)
			prefix = nil
		}
		if len(line) > 0 && (line[0] == ';' || line[0] == '#') {
			continue
		}
		f := strings.Fields(string(line))
		if len(f) < 1 {
			continue
		}
		switch f[0] {
		case "nameserver":
			if len(f) > 1 && len(conf.servers) < 3 {
				if _, err := netip.ParseAddr(f[1]); err == nil {
					conf.servers = append(conf.servers, net.JoinHostPort(f[1], "53"))
				}
			}
		case "domain":
			if len(f) > 1 {
				conf.search = []string{dns.Fqdn(f[1])}
			}

		case "search":
			conf.search = make([]string, 0, len(f)-1)
			for i := 1; i < len(f); i++ {
				name := dns.Fqdn(f[i])
				if name == "." {
					continue
				}
				conf.search = append(conf.search, name)
			}

		case "options":
			for _, s := range f[1:] {
				switch {
				case strings.HasPrefix(s, "ndots:"):
					n, _, _ := dtoi(s[6:])
					if n < 0 {
						n = 0
					} else if n > 15 {
						n = 15
					}
					conf.ndots = n
				case strings.HasPrefix(s, "timeout:"):
					n, _, _ := dtoi(s[8:])
					if n < 1 {
						n = 1
					}
					conf.timeout = time.Duration(n) * time.Second
				case strings.HasPrefix(s, "attempts:"):
					n, _, _ := dtoi(s[9:])
					if n < 1 {
						n = 1
					}
					conf.attempts = n
				case s == "rotate":
					conf.rotate = true
				case s == "single-request" || s == "single-request-reopen":
					conf.singleRequest = true
				case s == "use-vc" || s == "usevc" || s == "tcp":
					conf.useTCP = true
				case s == "trust-ad":
					conf.trustAD = true
				case s == "edns0":
				case s == "no-reload":
					conf.noReload = true
				default:
					conf.unknownOpt = true
				}
			}

		case "lookup":
			conf.lookup = f[1:]

		default:
			conf.unknownOpt = true
		}
	}
	if len(conf.servers) == 0 {
		conf.servers = defaultNS
	}
	if len(conf.search) == 0 {
		conf.search = dnsDefaultSearch()
	}
	return conf
}

type dnsConfig struct {
	servers       []string
	search        []string
	ndots         int
	timeout       time.Duration
	attempts      int
	rotate        bool
	unknownOpt    bool
	lookup        []string
	err           error
	mtime         time.Time
	soffset       uint32
	singleRequest bool
	useTCP        bool
	trustAD       bool
	noReload      bool
}

func (conf *dnsConfig) serverOffset() uint32 {
	if conf.rotate {
		return atomic.AddUint32(&conf.soffset, 1) - 1 // return 0 to start
	}
	return 0
}

func (conf *dnsConfig) nameList(name string) []string {
	l := len(name)
	rooted := l > 0 && name[l-1] == '.'
	if l > 254 || l == 254 && !rooted {
		return nil
	}

	if rooted {
		if avoidDNS(name) {
			return nil
		}
		return []string{name}
	}

	hasNdots := strings.Count(name, ".") >= conf.ndots
	name += "."
	// l++

	names := make([]string, 0, 1+len(conf.search))
	if hasNdots && !avoidDNS(name) {
		names = append(names, name)
	}
	for _, suffix := range conf.search {
		fqdn := name + suffix
		if !avoidDNS(fqdn) && len(fqdn) <= 254 {
			names = append(names, fqdn)
		}
	}
	if !hasNdots && !avoidDNS(name) {
		names = append(names, name)
	}
	return names
}

type resolverConfig struct {
	initOnce    sync.Once
	ch          chan struct{}
	lastChecked time.Time
	dnsConfig   atomic.Pointer[dnsConfig]
}

var resolvConf resolverConfig

func getSystemDNSConfig(ctx context.Context) *dnsConfig {
	resolvConf.tryUpdate(ctx, "/etc/resolv.conf")
	return resolvConf.dnsConfig.Load()
}

func (conf *resolverConfig) init(ctx context.Context) {
	conf.dnsConfig.Store(dnsReadConfig(ctx, "/etc/resolv.conf"))
	conf.lastChecked = time.Now()
	conf.ch = make(chan struct{}, 1)
}

func (conf *resolverConfig) tryUpdate(ctx context.Context, name string) {
	conf.initOnce.Do(func() {
		conf.init(ctx)
	})

	if conf.dnsConfig.Load().noReload {
		return
	}
	if !conf.tryAcquireSema() {
		return
	}
	defer conf.releaseSema()

	now := time.Now()
	if conf.lastChecked.After(now.Add(-5 * time.Second)) {
		return
	}
	conf.lastChecked = now
	if runtime.GOOS != "windows" {
		var mtime time.Time
		if fi, err := os.Stat(name); err == nil {
			mtime = fi.ModTime()
		}
		if mtime.Equal(conf.dnsConfig.Load().mtime) {
			return
		}
	}
	dnsConf := dnsReadConfig(ctx, name)
	conf.dnsConfig.Store(dnsConf)
}

func (conf *resolverConfig) tryAcquireSema() bool {
	select {
	case conf.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

func (conf *resolverConfig) releaseSema() {
	<-conf.ch
}
