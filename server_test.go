package gorums_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/dynamic"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

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
	gorums.NewNode(t, lis.Addr().String(), gorums.WithMetadata(md))

	select {
	case <-time.After(100 * time.Millisecond):
	case <-signal:
	}

	if message != "hello" {
		t.Errorf("incorrect message: got '%s', want 'hello'", message)
	}
}

func appendStringInterceptor(in, out string) gorums.Interceptor {
	return func(ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		// Check if msg and msg.message are not nil before using AsProto
		if msg != nil && msg.GetProtoMessage() != nil {
			if req := gorums.AsProto[proto.Message](msg); req != nil {
				dynamic.SetVal(req, dynamic.GetVal(req)+in)
			}
		}
		resp, err := next(ctx, msg)
		if resp != nil && resp.GetProtoMessage() != nil {
			if r := gorums.AsProto[proto.Message](resp); r != nil {
				dynamic.SetVal(r, dynamic.GetVal(r)+out)
			}
		}
		return resp, err
	}
}

type interceptorSrv struct{}

func (interceptorSrv) Test(_ gorums.ServerCtx, req proto.Message) (proto.Message, error) {
	return dynamic.NewResponse(dynamic.GetVal(req) + "server-"), nil
}

func TestServerInterceptorsChain(t *testing.T) {
	// set up a server with two interceptors: i1, i2
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		dynamic.Register(t)
		interceptorSrv := &interceptorSrv{}
		s := gorums.NewServer(gorums.WithInterceptors(
			appendStringInterceptor("i1in-", "i1out"),
			appendStringInterceptor("i2in-", "i2out-"),
		))
		// register final handler which appends "final-" to the request value
		s.RegisterHandler(dynamic.MockServerMethodName, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[proto.Message](in)
			resp, err := interceptorSrv.Test(ctx, req)
			return gorums.NewResponseMessage(in.GetMetadata(), resp), err
		})
		return s
	})
	defer teardown()

	node := gorums.NewNode(t, addrs[0])

	// call the RPC
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := node.RPCCall(ctx, gorums.CallData{
		Message: dynamic.NewRequest("client-"),
		Method:  dynamic.MockServerMethodName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("unexpected nil response")
	}
	r, ok := res.(proto.Message)
	if !ok {
		t.Fatalf("unexpected response type: %T", res)
	}
	want := "client-i1in-i2in-server-i2out-i1out"
	if dynamic.GetVal(r) != want {
		t.Fatalf("unexpected response value: got %q, want %q", dynamic.GetVal(r), want)
	}
}
