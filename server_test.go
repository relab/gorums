package gorums_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

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

func appendStringInterceptor(inStr, outStr string) gorums.Interceptor {
	return func(ctx gorums.ServerCtx, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		// update the underlying request gorums.Message's message field (pb.StringValue in this case)
		req.Value += inStr

		// TODO(meling): I did not like this change; I need to think more about this;
		//  can we make interceptors modify the request and responses in a cleaner way?
		//  Maybe we can add a method to stream.Message to modify/marshal the payload?
		//  Or maybe interceptors can operate on a InterceptorMessage wrapper that can
		//  hold the proto message that can be manipulated directly, and only on the way
		//  out of the interceptor chain, the wrapper is marshaled into the stream.Message.Payload.

		// re-marshal the modified request into the in message
		reqPayload, err := proto.Marshal(req)
		if err != nil {
			return nil, err
		}
		in.SetPayload(reqPayload)

		// call the next handler
		out, err := next(ctx, in)
		if err != nil {
			return nil, err
		}
		resp := gorums.AsProto[*pb.StringValue](out)
		// update the underlying response gorums.Message's message field (pb.StringValue in this case)
		resp.Value += outStr
		// re-marshal the modified response into the out message
		respPayload, err := proto.Marshal(resp)
		if err != nil {
			return nil, err
		}
		out.SetPayload(respPayload)
		return out, err
	}
}

type interceptorSrv struct{}

func (interceptorSrv) Test(_ gorums.ServerCtx, req *pb.StringValue) (*pb.StringValue, error) {
	return pb.String(req.GetValue() + "server-"), nil
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
			req := gorums.AsProto[*pb.StringValue](in)
			resp, err := interceptorSrv.Test(ctx, req)
			if err != nil {
				return nil, err
			}
			return gorums.NewResponseMessage(in, resp), nil
		})
		return s
	}
	node := gorums.TestNode(t, interceptorServerFn)

	ctx := gorums.TestContext(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	res, err := gorums.RPCCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String("client-"), mock.TestMethod)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("unexpected nil response")
	}
	want := "client-i1in-i2in-server-i2out-i1out"
	if res.GetValue() != want {
		t.Fatalf("unexpected response value: got %q, want %q", res.GetValue(), want)
	}
}

// TestTCPReconnection verifies that a node can reconnect after the
// underlying TCP connection is broken.
func TestTCPReconnection(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		return gorums.NewResponseMessage(in, req), nil
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
	t.Cleanup(gorums.Closer(t, mgr))

	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList([]string{addr}))
	if err != nil {
		t.Fatalf("NewConfiguration failed: %v", err)
	}
	node := cfg.Nodes()[0]

	// Send first message
	ctx := gorums.TestContext(t, time.Second)
	nodeCtx := node.Context(ctx)
	_, err = gorums.RPCCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String("1"), mock.TestMethod)
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
	nodeCtx2 := node.Context(ctx2)
	_, err = gorums.RPCCall[*pb.StringValue, *pb.StringValue](nodeCtx2, pb.String("2"), mock.TestMethod)
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
	srv2.RegisterHandler(mock.TestMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		return gorums.NewResponseMessage(in, req), nil
	})
	go func() {
		_ = srv2.Serve(lis2)
	}()
	defer srv2.Stop()

	// Wait for client backoff/reconnect
	time.Sleep(2 * time.Second)

	// Send message again
	ctx3 := gorums.TestContext(t, 2*time.Second)
	nodeCtx3 := node.Context(ctx3)
	_, err = gorums.RPCCall[*pb.StringValue, *pb.StringValue](nodeCtx3, pb.String("3"), mock.TestMethod)
	if err != nil {
		t.Errorf("Call after reconnection failed: %v", err)
	}
}
