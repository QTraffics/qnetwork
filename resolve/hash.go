package resolve

import (
	"hash/maphash"

	"github.com/miekg/dns"
)

var hashSeed = maphash.MakeSeed()

func hashQuestion(q dns.Question) uint32 {
	return uint32(maphash.Comparable(hashSeed, q))
}
