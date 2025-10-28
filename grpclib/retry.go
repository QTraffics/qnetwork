package grpclib

//type ClientStreamDaemon struct {
//	Name   string
//	Logger log.Logger
//
//	Backoff      backoff.Config
//	Stream       func(ctx context.Context) (grpc.ClientStream, error)
//	Handler      func(ctx context.Context, stream grpc.ClientStream) error
//	ErrorHandler ex.Handler
//}
//
//func (c *ClientStreamDaemon) Start(ctx context.Context) {
//	if c.Stream == nil {
//		panic("nil streamer")
//	}
//	if c.Handler == nil {
//		panic("nil handler")
//	}
//	if contextlib.Done(ctx) {
//		return
//	}
//
//	var (
//		logger  = values.UseDefaultNil(c.Logger, log.NOP)
//		backOff = values.UseDefault(c.Backoff, backoff.DefaultConfig)
//		name    = c.Name
//		stream  grpc.ClientStream
//		retries uint64
//	)
//	if name != "" {
//		name = "Daemon:" + name
//	}
//	logger = log.WithAttr(logger, log.NewMetadata("grpcs_daemon_key", slog.StringValue(name)))
//
//	logger.Info("new client stream daemon started")
//	for {
//		if contextlib.Done(ctx) {
//			logger.Debug("client stream daemon exited", log.AttrError(ctx.Err()))
//			return
//		}
//		var err error
//		if stream, err = c.Stream(ctx); err != nil {
//			dura := Backoff(backOff, retries)
//			logger.Error("stream create failed", slog.Duration("retry-in", dura/time.Second*time.Second))
//			select {
//			case <-ctx.Done():
//				logger.Info("exited", log.AttrError(ctx.Err()))
//				return
//			case <-time.After(dura):
//			}
//			retries++
//			logger.Debug("retry started", slog.Uint64("times", retries))
//			continue
//		}
//
//		if contextlib.Done(ctx) {
//			logger.Debug(context.Canceled.Error())
//			stream.CloseSend()
//			return
//		}
//
//		retries = 0
//		var (
//			quit bool
//		)
//		func() {
//			defer func() {
//				if stream != nil {
//					stream.CloseSend()
//				}
//				if err := recover(); err != nil {
//					logger.Error("handler panic !", slog.String(log.KeyError, fmt.Sprint(err)))
//				}
//			}()
//			err := c.Handler(ctx, stream)
//			if err == nil || ex.IsMulti(err, io.EOF) {
//				quit = true
//				return
//			}
//			if c.ErrorHandler != nil {
//				c.ErrorHandler.NewError(err)
//			} else {
//				logger.Warn("stream returned an unhandled error", log.AttrGRPCError(err))
//			}
//			quit = false
//		}()
//		if quit {
//			break
//		}
//	}
//}
