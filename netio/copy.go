package netio

import (
	"context"
	"net"

	"github.com/qtraffics/qtfra/enhancements/iolib"
	"github.com/qtraffics/qtfra/threads"
)

type closeWriter interface {
	CloseWrite() error
}

func CopyConn(ctx context.Context, source net.Conn, destination net.Conn) error {
	var (
		group threads.Group
	)

	if closer, ok := destination.(closeWriter); ok {
		group.Append("upload", func(ctx context.Context) error {
			_, err := iolib.Copy(source, destination)
			if err == nil {
				_ = closer.CloseWrite()
			} else {
				_ = iolib.Close(destination)
			}
			return err
		})
	} else {
		group.Append("upload", func(ctx context.Context) error {
			defer iolib.Close(destination)
			_, err := iolib.Copy(source, destination)
			return err
		})
	}
	if closer, ok := source.(closeWriter); ok {
		group.Append("download", func(ctx context.Context) error {
			_, err := iolib.Copy(destination, source)
			if err == nil {
				_ = closer.CloseWrite()
			} else {
				_ = iolib.Close(source)
			}
			return err
		})
	} else {
		group.Append("download", func(ctx context.Context) error {
			defer iolib.Close(source)
			_, err := iolib.Copy(destination, source)
			return err
		})
	}

	group.Cleanup(func() {
		_ = iolib.Close(source, destination)
	})

	return group.Run(ctx)
}
