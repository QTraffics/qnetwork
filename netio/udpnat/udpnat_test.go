package udpnat

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qtfra/buf"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPacketWriter struct {
	size int
}

func (m *mockPacketWriter) WriteTo(bs []byte, destination net.Addr) (n int, err error) {
	m.size += len(bs)
	return len(bs), nil
}

func TestCreate(t *testing.T) {
	backWriter := &mockPacketWriter{}
	source := addrs.FromAddrPort(netip.MustParseAddrPort("1.1.1.1:53"))
	mockData := buf.As([]byte("test data"))

	nat := New(backWriter, nil)
	defer nat.Close()

	conn, isNew := nat.NewPacket(mockData, source, addrs.Socksaddr{})
	require.True(t, isNew)
	require.NotNil(t, conn)
	nn, err := conn.Write(mockData.Bytes())
	require.NoError(t, err)

	assert.Equal(t, mockData.Len(), nn)
	assert.Equal(t, backWriter.size, nn)
}

func TestMulti(t *testing.T) {
	packetWriter := &mockPacketWriter{}
	const fixedTestData = "test data"
	type Case struct {
		Source  addrs.Socksaddr
		Handler PacketHandler
		Data    []*buf.Buffer

		OnConn func(t *testing.T, c *natConn)
	}
	cases := []Case{
		{
			Source: addrs.FromAddrPort(netip.MustParseAddrPort("1.1.1.1:53")),
			Data:   []*buf.Buffer{buf.As([]byte(fixedTestData))},
			OnConn: func(t *testing.T, c *natConn) {
				packetWriter.size = 0
				nn, err := c.Write([]byte(fixedTestData))
				require.NoError(t, err)
				assert.Equal(t, len(fixedTestData), nn)
				assert.Equal(t, len(fixedTestData), packetWriter.size)
			},
		},
		{
			Source: addrs.FromAddrPort(netip.MustParseAddrPort("1.1.1.1:53")),
			Data:   []*buf.Buffer{buf.As([]byte(fixedTestData)), buf.As([]byte(fixedTestData))},
			OnConn: func(t *testing.T, c *natConn) {
				readBuffer := buf.NewHuge()
				defer readBuffer.Free()
				for i := range 3 {
					exceptData := strings.Repeat(fixedTestData, i+1)
					n, err := readBuffer.ReadFromOnce(c)
					require.NoError(t, err)
					assert.Equal(t, len(fixedTestData), n)
					assert.Equal(t, len(exceptData), readBuffer.Len())
					assert.Equal(t, exceptData, string(readBuffer.Bytes()))
				}
			},
		},
	}
	udpnat := New(packetWriter, nil)
	defer udpnat.Close()

	for _, c := range cases {
		var conn Conn
		for _, d := range c.Data {
			conn, _ = udpnat.NewPacket(d, c.Source, addrs.FromAddrPort(netip.AddrPort{}))
		}
		if c.Handler != nil {
			conn.SetHandler(c.Handler)
		}
		c.OnConn(t, conn.(*natConn))
	}
}
