package gorums_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/tests/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestServerCallback(t *testing.T) {
	var message string
	signal := make(chan struct{})

	srv := gorums.NewServer(gorums.WithConnectCallback(func(ctx context.Context) {
		m, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return
		}
		message = m.Get("message")[0]
		signal <- struct{}{}
	}))

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	md := metadata.New(map[string]string{"message": "hello"})

	mgr := gorums.NewRawManager(
		gorums.WithMetadata(md),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	node, err := gorums.NewRawNode(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	select {
	case <-time.After(100 * time.Millisecond):
	case <-signal:
	}

	if message != "hello" {
		t.Errorf("incorrect message: got '%s', want 'hello'", message)
	}
}

type interceptorSrv struct{}

func (interceptorSrv) Test(_ gorums.ServerCtx, req *mock.Request) (*mock.Response, error) {
	return mock.Response_builder{Val: req.GetVal() + "server-"}.Build(), nil
}

func appendStringInterceptor(in, out string) gorums.Interceptor {
	return func(ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		if req := gorums.AsProto[*mock.Request](msg); req != nil {
			req.SetVal(req.GetVal() + in)
		}
		resp, err := next(ctx, msg)
		if resp != nil {
			if r := gorums.AsProto[*mock.Response](resp); r != nil {
				r.SetVal(r.GetVal() + out)
			}
		}
		return resp, err
	}
}

func TestServerInterceptorsChain(t *testing.T) {
	// set up a server with two interceptors: i1, i2
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		interceptorSrv := &interceptorSrv{}
		s := gorums.NewServer(gorums.WithInterceptors(
			appendStringInterceptor("i1in-", "i1out"),
			appendStringInterceptor("i2in-", "i2out-"),
		))
		// register final handler which appends "final-" to the request value
		s.RegisterHandler("mock.Server.Test", func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*mock.Request](in)
			resp, err := interceptorSrv.Test(ctx, req)
			return gorums.NewResponseMessage(in.GetMetadata(), resp), err
		})
		return s
	})
	defer teardown()

	mgr := gorums.NewRawManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	node, err := gorums.NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	// call the RPC
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.Nodes()[0].RPCCall(ctx, gorums.CallData{
		Message: mock.Request_builder{Val: "client-"}.Build(),
		Method:  "mock.Server.Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("unexpected nil response")
	}
	r, ok := res.(*mock.Response)
	if !ok {
		t.Fatalf("unexpected response type: %T", res)
	}
	want := "client-i1in-i2in-server-i2out-i1out"
	if r.GetVal() != want {
		t.Fatalf("unexpected response value: got %q, want %q", r.GetVal(), want)
	}
}
