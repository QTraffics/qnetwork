package meta

import "strconv"

type NetworkVersion int

const (
	NetworkFamily4 = "4"
	NetworkFamily6 = "6"
)

const (
	NetworkVersion4    = 4
	NetworkVersion6    = 6
	NetworkVersionDual = 0
)

var (
	NetworkUDP  = Network{ProtocolUDP, NetworkVersionDual}
	NetworkUDP4 = Network{ProtocolUDP, NetworkVersion4}
	NetworkUDP6 = Network{ProtocolUDP, NetworkVersion6}
	NetworkTCP  = Network{ProtocolTCP, NetworkVersionDual}
	NetworkTCP4 = Network{ProtocolTCP, NetworkVersion4}
	NetworkTCP6 = Network{ProtocolTCP, NetworkVersion6}
)

type Network struct {
	Protocol Protocol
	Version  NetworkVersion
}

func (n Network) Is4() bool {
	return n.Version == NetworkVersionDual || n.Version == NetworkVersion6
}

func (n Network) Is6() bool {
	return n.Version == NetworkVersionDual || n.Version == NetworkVersion4
}

func (n Network) IsUDP() bool {
	return n.Protocol == ProtocolUDP
}

func (n Network) IsTCP() bool {
	return n.Protocol == ProtocolTCP
}

func ParseNetwork(network string) (Network, bool) {
	nn := Network{}
	switch network {
	case "tcp", "udp":
		nn.Version = NetworkVersionDual
		nn.Protocol = Protocol(network)
	case "tcp4", "udp4":
		nn.Version = NetworkVersion4
		nn.Protocol = Protocol(network[:3])
	case "tcp6", "udp6":
		nn.Version = NetworkVersion6
		nn.Protocol = Protocol(network[:3])
	default:
		return Network{}, false
	}
	return nn, true
}

func (n Network) IsValid() bool {
	return n.Protocol.IsValid() &&
		(n.Version == NetworkVersionDual ||
			n.Version == NetworkVersion4 ||
			n.Version == NetworkVersion6)
}

func (n Network) ApplyStrategy(strategy Strategy) Network {
	if strategy == StrategyIPv4Only {
		n.Version = NetworkVersion4
	}
	if strategy == StrategyIPv6Only {
		n.Version = NetworkVersion6
	}
	return n
}

func (n Network) String() string {
	switch n.Protocol {
	case ProtocolUDP:
		switch n.Version {
		case NetworkVersion4:
			return "udp4"
		case NetworkVersion6:
			return "udp6"
		case NetworkVersionDual:
			return "udp"
		}
	case ProtocolTCP:
		switch n.Version {
		case NetworkVersion4:
			return "tcp4"
		case NetworkVersion6:
			return "tcp6"
		case NetworkVersionDual:
			return "tcp"
		}
	}
	// slow path
	return n.Protocol.String() + strconv.Itoa(int(n.Version))
}
