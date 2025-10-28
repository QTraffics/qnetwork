package httplib

import (
	"context"
	"net"

	"github.com/QTraffics/qtfra/registry"
)

type HTTPServerConn net.Conn

func ServerConnContext() func(ctx context.Context, conn net.Conn) context.Context {
	return func(ctx context.Context, conn net.Conn) context.Context {
		return registry.ContextWith[HTTPServerConn](ctx, conn)
	}
}

func GetServerConnFromContext(ctx context.Context) net.Conn {
	v, ok := registry.ContextFrom[HTTPServerConn](ctx)
	if ok {
		return v
	}
	return nil
}
