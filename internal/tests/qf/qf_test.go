package qf

import (
	"context"
	"fmt"
	"testing"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const requestValue = 0x1001_1001

type testSrv struct{}

func (s testSrv) UseReq(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{Result: req.GetValue()}.Build(), nil
}

// TODO(meling): we only need one method now
// TODO(meling): consider using the mock.Test server and more this benchmark to the root folder.
func (s testSrv) IgnoreReq(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{Result: req.GetValue()}.Build(), nil
}

// TODO(meling): repurpose this benchmark to benchmark various quorum functions via gorums.WithQuorumFunc
// - default quorum function, various chains of quorum function iterators,
// - custom quorum function directly as iterators vs via the legacy QuorumSpecInterceptor helper
func BenchmarkFullStackQF(b *testing.B) {
	for n := 3; n < 20; n += 2 {
		addrs, stop := gorums.TestSetup(b, n, func(_ int) gorums.ServerIface {
			srv := gorums.NewServer()
			RegisterQuorumFunctionServer(srv, &testSrv{})
			return srv
		})
		mgr := gorums.NewRawManager(
			gorums.WithGrpcDialOptions(
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			),
		)
		c, err := gorums.NewRawConfiguration(mgr,
			gorums.WithNodeList(addrs),
		)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()

		b.Run(fmt.Sprintf("UseReq_%d", n), func(b *testing.B) {
			qf := func(ctx *gorums.ClientCtx[*Request, *Response]) (*Response, error) {
				quorum := ctx.Size()/2 + 1
				replies := ctx.Responses().IgnoreErrors().CollectAll()
				if len(replies) < quorum {
					return nil, fmt.Errorf("incomplete call: %d replies", len(replies))
				}
				_ = ctx.Request().GetValue()
				var reply *Response
				for _, r := range replies {
					reply = r
					break
				}
				return reply, nil
			}
			cfgCtx := gorums.WithConfigContext(context.Background(), c)
			for b.Loop() {
				resp, err := UseReq(cfgCtx,
					Request_builder{Value: int64(requestValue)}.Build(),
					gorums.WithQuorumFunc(qf))
				if err != nil {
					b.Fatalf("UseReq error: %v", err)
				}
				_ = resp.GetResult()
			}
		})
		b.Run(fmt.Sprintf("IgnoreReq_%d", n), func(b *testing.B) {
			qf := func(ctx *gorums.ClientCtx[*Request, *Response]) (*Response, error) {
				quorum := ctx.Size()/2 + 1
				replies := ctx.Responses().IgnoreErrors().CollectAll()
				if len(replies) < quorum {
					return nil, fmt.Errorf("incomplete call: %d replies", len(replies))
				}
				var reply *Response
				for _, r := range replies {
					reply = r
					break
				}
				return reply, nil
			}
			cfgCtx := gorums.WithConfigContext(context.Background(), c)
			for b.Loop() {
				resp, err := IgnoreReq(cfgCtx,
					Request_builder{Value: int64(requestValue)}.Build(),
					gorums.WithQuorumFunc(qf))
				if err != nil {
					b.Fatalf("IgnoreReq error: %v", err)
				}
				_ = resp.GetResult()
			}
		})
		// close manager and stop gRPC servers;
		// must be done for each iteration
		mgr.Close()
		stop()
	}
}
