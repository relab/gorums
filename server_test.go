package gorums_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

func TestServerCallback(t *testing.T) {
	var message string
	signal := make(chan struct{})

	srvOption := gorums.WithConnectCallback(func(ctx context.Context) {
		m, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return
		}
		message = m.Get("message")[0]
		signal <- struct{}{}
	})
	mgrOption := gorums.WithMetadata(metadata.New(map[string]string{"message": "hello"}))

	gorums.TestNode(t, nil, srvOption, mgrOption)

	select {
	case <-time.After(100 * time.Millisecond):
	case <-signal:
	}

	if message != "hello" {
		t.Errorf("incorrect message: got '%s', want 'hello'", message)
	}
}

func appendStringInterceptor(in, out string) gorums.Interceptor {
	return func(ctx gorums.ServerCtx, inMsg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		if req := inMsg.GetProtoMessage(); req != nil {
			// update the underlying request gorums.Message's proto.Message
			mock.SetVal(req, mock.GetVal(req)+in)
		}
		// call the next handler
		outMsg, err := next(ctx, inMsg)
		if outMsg != nil {
			if resp := outMsg.GetProtoMessage(); resp != nil {
				// update the underlying response gorums.Message's proto.Message
				mock.SetVal(resp, mock.GetVal(resp)+out)
			}
		}
		return outMsg, err
	}
}

type interceptorSrv struct{}

func (interceptorSrv) Test(_ gorums.ServerCtx, req proto.Message) (proto.Message, error) {
	return pb.String(mock.GetVal(req) + "server-"), nil
}

func TestServerInterceptorsChain(t *testing.T) {
	// set up a server with two interceptors: i1, i2
	interceptorServerFn := func(_ int) gorums.ServerIface {
		interceptorSrv := &interceptorSrv{}
		s := gorums.NewServer(gorums.WithInterceptors(
			appendStringInterceptor("i1in-", "i1out"),
			appendStringInterceptor("i2in-", "i2out-"),
		))
		// register final handler which appends "final-" to the request value
		s.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			resp, err := interceptorSrv.Test(ctx, in.GetProtoMessage())
			return gorums.NewResponseMessage(in.GetMetadata(), resp), err
		})
		return s
	}
	node := gorums.TestNode(t, interceptorServerFn)

	ctx := gorums.TestContext(t, 5*time.Second)
	nodeCtx := gorums.WithNodeContext(ctx, node)
	res, err := gorums.RPCCall(nodeCtx, pb.String("client-"), mock.TestMethod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("unexpected nil response")
	}
	want := "client-i1in-i2in-server-i2out-i1out"
	if mock.GetVal(res) != want {
		t.Fatalf("unexpected response value: got %q, want %q", mock.GetVal(res), want)
	}
}

// TestTCPReconnection verifies that a node can reconnect after the
// underlying TCP connection is broken.
func TestTCPReconnection(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in.GetMetadata(), in.GetProtoMessage()), nil
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := lis.Addr().String()

	go func() {
		_ = srv.Serve(lis)
	}()

	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(mgr.Close)

	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList([]string{addr}))
	if err != nil {
		t.Fatalf("NewConfiguration failed: %v", err)
	}
	node := cfg.Nodes()[0]

	// Send first message
	ctx := gorums.TestContext(t, time.Second)
	nodeCtx := gorums.WithNodeContext(ctx, node)
	_, err = gorums.RPCCall(nodeCtx, pb.String("1"), mock.TestMethod)
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	// Stop server
	srv.Stop()
	lis.Close()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Sending now should fail or timeout
	ctx2 := gorums.TestContext(t, 200*time.Millisecond)
	nodeCtx2 := gorums.WithNodeContext(ctx2, node)
	_, err = gorums.RPCCall(nodeCtx2, pb.String("2"), mock.TestMethod)
	if err == nil {
		// It might succeed if it just queued it? But we wait for response.
	} else {
		t.Logf("Got expected error during downtime: %v", err)
	}

	// Restart server
	lis2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("Could not re-bind to %s: %v", addr, err)
	}

	srv2 := gorums.NewServer()
	srv2.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in.GetMetadata(), in.GetProtoMessage()), nil
	})
	go func() {
		_ = srv2.Serve(lis2)
	}()
	defer srv2.Stop()

	// Wait for client backoff/reconnect
	time.Sleep(2 * time.Second)

	// Send message again
	ctx3 := gorums.TestContext(t, 2*time.Second)
	nodeCtx3 := gorums.WithNodeContext(ctx3, node)
	_, err = gorums.RPCCall(nodeCtx3, pb.String("3"), mock.TestMethod)
	if err != nil {
		t.Errorf("Call after reconnection failed: %v", err)
	}
}
