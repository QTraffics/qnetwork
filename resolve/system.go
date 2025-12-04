package resolve

import "github.com/qtraffics/qnetwork/resolve/transport"

var SystemClient = &TransportClient{
	HeadlessClient: NewHeadlessClient(NewCache()),
	Transport:      transport.NewLocalTransport(nil),
}
