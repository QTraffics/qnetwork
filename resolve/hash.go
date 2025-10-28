package resolve

import (
	"encoding/binary"

	"github.com/QTraffics/qnetwork/addrs"
	"github.com/QTraffics/qtfra/enhancements/pool"
	"github.com/cespare/xxhash/v2"

	"github.com/miekg/dns"
)

var hashPool = pool.New(func() *xxhash.Digest {
	return xxhash.New()
})

func hashQuestion(q dns.Question) uint32 {
	if len(q.Name) > addrs.MaxFqdnLength {
		panic("invalid dns question")
	}

	h := hashPool.Get()
	_, _ = h.Write([]byte{byte(len(q.Name))})
	_, _ = h.WriteString(q.Name)

	var buf [4]byte
	binary.BigEndian.PutUint16(buf[:2], q.Qtype)
	binary.BigEndian.PutUint16(buf[2:], q.Qclass)
	_, _ = h.Write(buf[:])
	ret := uint32(h.Sum64())
	h.Reset()
	hashPool.Put(h)
	return ret
}
