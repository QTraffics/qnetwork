package udpnat

import (
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/qtraffics/qnetwork/addrs"
	"github.com/qtraffics/qnetwork/netio"
	"github.com/qtraffics/qtfra/buf"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const test = "test data"

type mockPacketWriter struct {
	size int
}

func (m *mockPacketWriter) WriteTo(bs []byte, destination net.Addr) (n int, err error) {
	m.size += len(bs)
	return len(bs), nil
}

type natCases struct {
	Source       addrs.Socksaddr
	PacketWriter netio.PacketWriter
	Handler      PacketHandler

	BuildData func(c natCases) []*buf.Buffer
	Data      []*buf.Buffer

	OnConn  func(t *testing.T, c *natConn)
	OnClose func(c Conn)
}

func (c natCases) Exec(t *testing.T, u *UdpNat) {
	var conn Conn
	data := c.Data
	if c.BuildData != nil {
		data = append(data, c.BuildData(c)...)
	}

	for _, d := range data {
		conn, _ = u.NewPacket(d, c.Source, addrs.FromAddrPort(netip.AddrPort{}))
	}

	if c.Handler != nil {
		conn.SetHandler(c.Handler)
	}
	c.OnConn(t, conn.(*natConn))
}

func TestCreate(t *testing.T) {
	backWriter := &mockPacketWriter{}
	source := addrs.FromAddrPort(netip.MustParseAddrPort("1.1.1.1:53"))
	mockData := buf.As([]byte(test))

	nat, err := New(func(source addrs.Socksaddr, destination addrs.Socksaddr, p netio.UDPPacket) PrepareResult {
		return PrepareResult{
			Success:      true,
			PacketWriter: &netio.BindPacketWriter{PacketWriter: backWriter, Destination: source},
		}
	}, nil)

	require.NoError(t, err)
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
	cases := []natCases{
		{
			Source:       addrs.FromAddrPort(netip.MustParseAddrPort("1.1.1.1:53")),
			Data:         []*buf.Buffer{buf.As([]byte(test))},
			PacketWriter: packetWriter,
			OnConn: func(t *testing.T, c *natConn) {
				packetWriter.size = 0
				nn, err := c.Write([]byte(test))
				require.NoError(t, err)
				assert.Equal(t, len(test), nn)
				assert.Equal(t, len(test), packetWriter.size)
			},
		},
		{
			Source:       addrs.FromAddrPort(netip.MustParseAddrPort("1.1.1.1:53")),
			Data:         []*buf.Buffer{buf.As([]byte(test)), buf.As([]byte(test))},
			PacketWriter: packetWriter,
			OnConn: func(t *testing.T, c *natConn) {
				readBuffer := buf.NewHuge()
				defer readBuffer.Free()
				for i := range 3 {
					exceptData := strings.Repeat(test, i+1)
					n, err := readBuffer.ReadFromOnce(c)
					require.NoError(t, err)
					assert.Equal(t, len(test), n)
					assert.Equal(t, len(exceptData), readBuffer.Len())
					assert.Equal(t, exceptData, string(readBuffer.Bytes()))
				}
			},
		},
	}

	udpnat, err := New(func(source addrs.Socksaddr, destination addrs.Socksaddr, p netio.UDPPacket) PrepareResult {
		for _, c := range cases {
			if c.Source != source {
				continue
			}
			return PrepareResult{
				Success:      true,
				OnClose:      c.OnClose,
				Handler:      c.Handler,
				PacketWriter: c.PacketWriter,
			}
		}
		return PrepareResult{}
	}, nil)

	require.NoError(t, err)
	defer udpnat.Close()

	for _, c := range cases {
		c.Exec(t, udpnat)
	}
}
