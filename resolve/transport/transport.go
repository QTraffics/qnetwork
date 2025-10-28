package transport

import (
	"context"

	"github.com/miekg/dns"
)

type Transport interface {
	Exchange(ctx context.Context, message *dns.Msg) (*dns.Msg, error)
}
