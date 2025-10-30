package addrs

import (
	_ "unsafe"

	"github.com/miekg/dns"
)

// for linkname

//go:linkname IsDomainName net.isDomainName
func IsDomainName(domain string) bool

var (
	Fqdn   = dns.Fqdn
	IsFqdn = dns.IsFqdn
)

const (
	MaxFqdnLength = 255
)
