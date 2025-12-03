package gorums_test

import (
	"context"
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
