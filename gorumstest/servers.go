package gorumstest

import (
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/servers"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// ServerIface is the interface a server must implement to be started by
// [Config], [Node], or [Servers]. [gorums.Server] implements it, as does a
// gRPC server.
type ServerIface = servers.ServerIface

// newDefaultServer creates the mock server that [Config], [Node], and
// [Servers] use when their srvFn argument is nil, with the given server
// options.
func newDefaultServer(i int, opts ...gorums.ServerOption) ServerIface {
	srv := gorums.NewServer(opts...)
	ts := testSrv{val: int32((i + 1) * 10)}
	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		resp, err := ts.Test(ctx, req)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	srv.RegisterHandler(mock.GetValueMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.Int32Value](in)
		resp, err := ts.GetValue(ctx, req)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	return srv
}

type testSrv struct {
	val int32
}

func (testSrv) Test(_ gorums.ServerContext, _ *pb.StringValue) (*pb.StringValue, error) {
	return pb.String(""), nil
}

func (ts testSrv) GetValue(_ gorums.ServerContext, _ *pb.Int32Value) (*pb.Int32Value, error) {
	return pb.Int32(ts.val), nil
}

// EchoHandler returns a handler that replies to a string-valued request with
// prefix+": "+value.
func EchoHandler(prefix string) gorums.Handler {
	return func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		return gorums.NewResponseMessage(in, pb.String(prefix+": "+req.GetValue())), nil
	}
}

// EchoServer returns a server that echoes back its request, prefixed with
// "echo: ", suitable for use as the srvFn argument to [Config],
// [Node], or [Servers].
func EchoServer(_ int) ServerIface {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, EchoHandler("echo"))
	return srv
}

// StreamServer returns a srvFn for [Config], [Node], or [Servers] whose
// servers respond to a request with three echoed responses, each followed by
// delay. A zero delay sends the responses back-to-back.
func StreamServer(delay time.Duration) func(int) ServerIface {
	return func(int) ServerIface {
		srv := gorums.NewServer()
		srv.RegisterHandler(mock.StreamMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*pb.StringValue](in)
			val := req.GetValue()

			// Send 3 responses
			for i := 1; i <= 3; i++ {
				resp := pb.String(fmt.Sprintf("echo: %s-%d", val, i))
				out := gorums.NewResponseMessage(in, resp)
				ctx.SendMessage(out)
				if delay > 0 {
					time.Sleep(delay)
				}
			}
			return nil, nil
		})
		return srv
	}
}
