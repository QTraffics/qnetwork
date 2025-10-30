package resolve

import (
	"github.com/qtraffics/qnetwork/resolve/transport"
	"github.com/qtraffics/qtfra/enhancements/slicelib"
	"github.com/qtraffics/qtfra/ex"

	"github.com/miekg/dns"
)

func CalculateTTL(message *dns.Msg) (ttl uint32) {
	for _, rrs := range [][]dns.RR{message.Answer, message.Ns, message.Extra} {
		for _, rr := range rrs {
			if ttl == 0 || ttl > rr.Header().Ttl {
				ttl = rr.Header().Ttl
			}
		}
	}
	return ttl
}

func OverwriteTTL(message *dns.Msg, ttl uint32) {
	for _, rrs := range [][]dns.RR{message.Answer, message.Ns, message.Extra} {
		for _, rr := range rrs {
			rr.Header().Ttl = ttl
		}
	}
}

func EdnsBackwards(req *dns.Msg, resp *dns.Msg) *dns.Msg {
	requestEdns0 := req.IsEdns0()
	responseEdns0 := resp.IsEdns0()
	if responseEdns0 != nil && (requestEdns0 == nil || requestEdns0.Version() < responseEdns0.Version()) {
		resp.Extra = slicelib.Filter(resp.Extra, func(it dns.RR) bool {
			return it.Header().Rrtype != dns.TypeOPT
		})
		if requestEdns0 != nil {
			resp.SetEdns0(requestEdns0.UDPSize(), responseEdns0.Do())
		}
	}
	return resp
}

func NewDNSRecordError(rcode int) error {
	return ex.New("rcode: ", dns.RcodeToString[rcode])
}

var FixedResponse = transport.FixedResponse
