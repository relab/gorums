package gorumstest

import (
	"fmt"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// DefaultServer returns a mock server implementation suitable for use as
// the srvFn argument to [Config], [Node], or [Servers].
func DefaultServer(i int) gorums.ServerIface {
	return defaultTestServer(i)
}

// defaultTestServer creates a test server with optional server options.
// This is the internal implementation used by both DefaultServer and
// the test framework when server options are provided.
func defaultTestServer(i int, opts ...gorums.ServerOption) gorums.ServerIface {
	srv := gorums.NewServer(opts...)
	ts := testSrv{val: int32((i + 1) * 10)}
	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		resp, err := ts.Test(ctx, req)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	srv.RegisterHandler(mock.GetValueMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
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

func (testSrv) Test(_ gorums.ServerCtx, _ *pb.StringValue) (*pb.StringValue, error) {
	return pb.String(""), nil
}

func (ts testSrv) GetValue(_ gorums.ServerCtx, _ *pb.Int32Value) (*pb.Int32Value, error) {
	return pb.Int32(ts.val), nil
}

// EchoServerFn returns a server that echoes back its request, prefixed with
// "echo: ", suitable for use as the srvFn argument to [Config],
// [Node], or [Servers].
func EchoServerFn(_ int) gorums.ServerIface {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		resp, err := echoSrv{}.Test(ctx, req)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})

	return srv
}

// echoSrv implements a simple echo server handler for testing
type echoSrv struct{}

func (echoSrv) Test(_ gorums.ServerCtx, req *pb.StringValue) (*pb.StringValue, error) {
	return pb.String("echo: " + req.GetValue()), nil
}

// StreamServerFn returns a server that responds to a request with three
// echoed responses, ten milliseconds apart, suitable for use as the srvFn
// argument to [Config], [Node], or [Servers].
func StreamServerFn(_ int) gorums.ServerIface {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.Stream, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		val := req.GetValue()

		// Send 3 responses
		for i := 1; i <= 3; i++ {
			resp := pb.String(fmt.Sprintf("echo: %s-%d", val, i))
			out := gorums.NewResponseMessage(in, resp)
			ctx.SendMessage(out)
			time.Sleep(10 * time.Millisecond)
		}
		return nil, nil
	})
	return srv
}

// StreamBenchmarkServerFn returns a server that responds to a request with
// three echoed responses sent back-to-back, without the delay
// [StreamServerFn] adds between responses, suitable for use as the srvFn
// argument to [Config], [Node], or [Servers].
func StreamBenchmarkServerFn(_ int) gorums.ServerIface {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.Stream, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		val := req.GetValue()

		// Send 3 responses
		for i := 1; i <= 3; i++ {
			resp := pb.String(fmt.Sprintf("echo: %s-%d", val, i))
			out := gorums.NewResponseMessage(in, resp)
			ctx.SendMessage(out)
		}
		return nil, nil
	})
	return srv
}
