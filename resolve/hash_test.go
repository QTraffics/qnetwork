package resolve

import (
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"testing"

	"github.com/qtraffics/qtfra/buf"
	"github.com/qtraffics/qtfra/enhancements/pool"

	"github.com/cespare/xxhash/v2"
	"github.com/miekg/dns"
)

var hashPool = pool.New[*xxhash.Digest](func() *xxhash.Digest {
	return xxhash.New()
})

var alwaysFalse bool

func hashQuestionWithPool(q dns.Question) uint32 {
	h := hashPool.Get()
	_, _ = h.Write([]byte{byte(len(q.Name))})
	_, _ = h.WriteString(q.Name)

	var buffer [4]byte
	binary.BigEndian.PutUint16(buffer[:2], q.Qtype)
	binary.BigEndian.PutUint16(buffer[2:], q.Qclass)
	_, _ = h.Write(buffer[:])
	ret := uint32(h.Sum64())
	h.Reset()
	hashPool.Put(h)
	return ret
}

func hashQuestionBuffer(q dns.Question) uint32 {
	buffer := buf.NewMinimal()
	defer buffer.Free()
	_, _ = buffer.WriteString(q.Name)
	binary.BigEndian.PutUint16(buffer.FreeBytes()[:], q.Qtype)
	buffer.Truncated(len(q.Name) + 2)
	binary.BigEndian.PutUint16(buffer.FreeBytes()[:], q.Qclass)
	buffer.Truncated(len(q.Name) + 4)

	return uint32(xxhash.Sum64(buffer.Bytes()))
}

// goos: linux
// goarch: amd64
// pkg: github.com/qtraffics/qnetwork/resolve
// cpu: AMD Ryzen 9 7940H w/ Radeon 780M Graphics
// BenchmarkHash/Digest_no_pool-16                 47182995                24.69 ns/op            0 B/op          0 allocs/op
// BenchmarkHash/Digest-16                         39087026                29.58 ns/op            0 B/op          0 allocs/op
// BenchmarkHash/Buffer-16                         22290589                55.88 ns/op           64 B/op          1 allocs/op
// PASS
func BenchmarkHash(b *testing.B) {
	question := dns.Question{
		Name:   "www.google.com",
		Qtype:  dns.TypeA,
		Qclass: dns.ClassINET,
	}
	b.Run("Digest_no_pool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = hashQuestion(question)
		}
	})
	b.Run("Digest", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = hashQuestionWithPool(question)
		}
	})
	b.Run("Buffer", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = hashQuestionBuffer(question)
		}
	})
}

// goos: linux
// goarch: amd64
// pkg: github.com/qtraffics/qnetwork/resolve
// cpu: AMD Ryzen 9 7940H w/ Radeon 780M Graphics
// BenchmarkMapHash/xxhash-16              46237074                21.77 ns/op            0 B/op          0 allocs/op
// BenchmarkMapHash/maphash-16             141870160                8.494 ns/op           0 B/op          0 allocs/op
// PASS
func BenchmarkMapHash(b *testing.B) {
	question := dns.Question{
		Name:   "www.google.com",
		Qtype:  dns.TypeA,
		Qclass: dns.ClassINET,
	}
	xx := xxhash.New()
	seed := maphash.MakeSeed()
	b.Run("xxhash", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = xx.Write([]byte{byte(len(question.Name))})
			_, _ = xx.WriteString(question.Name)

			var buffer [4]byte
			binary.BigEndian.PutUint16(buffer[:2], question.Qtype)
			binary.BigEndian.PutUint16(buffer[2:], question.Qclass)
			_, _ = xx.Write(buffer[:])
			v := uint32(xx.Sum64())

			if alwaysFalse {
				fmt.Print(v)
			}
			xx.Reset()
		}
	})
	b.Run("maphash", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			v := uint32(maphash.Comparable(seed, question))
			if alwaysFalse {
				fmt.Print(v)
			}
		}
	})
}
