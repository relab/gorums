package qf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
)

const requestValue = 0x1001_1001

type testQSpec struct {
	quorum int
}

func (q testQSpec) UseReqQF(in *Request, replies []*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	expected := in.GetValue()
	for _, reply := range replies {
		if expected != reply.GetResult() {
			return reply, true
		}
	}
	return replies[0], true
}

func (q testQSpec) IgnoreReqQF(_ *Request, replies []*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	expected := int64(requestValue)
	for _, reply := range replies {
		if expected != reply.GetResult() {
			return reply, true
		}
	}
	return replies[0], true
}

func (q testQSpec) WithoutReqQF(replies []*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	expected := int64(requestValue)
	for _, reply := range replies {
		if expected != reply.GetResult() {
			return reply, true
		}
	}
	return replies[0], true
}

func BenchmarkQF(b *testing.B) {
	for n := 3; n < 20; n += 2 {
		quorum := n / 2
		qspec := &testQSpec{quorum: quorum}
		request := &Request{Value: 1}

		b.Run(fmt.Sprintf("UseReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make([]*Response, 0, n)
				for j := 0; j < n; j++ {
					replies = append(replies, &Response{Result: request.Value})
					resp, q := qspec.UseReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("IgnoreReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make([]*Response, 0, n)
				for j := 0; j < n; j++ {
					replies = append(replies, &Response{Result: request.Value})
					resp, q := qspec.IgnoreReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("WithoutReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				replies := make([]*Response, 0, n)
				for j := 0; j < n; j++ {
					replies = append(replies, &Response{Result: request.Value})
					resp, q := qspec.WithoutReqQF(replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
	}
}

type testSrv struct {
	UnimplementedQuorumFunctionServer
}

func (s testSrv) UseReq(_ context.Context, req *Request) (*Response, error) {
	return &Response{Result: req.GetValue()}, nil
}

func (s testSrv) IgnoreReq(_ context.Context, req *Request) (*Response, error) {
	return &Response{Result: req.GetValue()}, nil
}

func BenchmarkFullStackQF(b *testing.B) {
	for n := 3; n < 20; n += 2 {
		srvAdrs, stop := gorums.TestSetup(b, n, func(srv *grpc.Server) {
			RegisterQuorumFunctionServer(srv, &testSrv{})
		})
		c, closeFn, err := NewConfig(srvAdrs, &testQSpec{quorum: n / 2},
			WithGrpcDialOptions(grpc.WithInsecure()),
			WithDialTimeout(10*time.Second),
		)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()

		b.Run(fmt.Sprintf("UseReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				resp, err := c.UseReq(context.Background(), &Request{Value: int64(requestValue)})
				if err != nil {
					b.Fatalf("UseReq error: %v", err)
				}
				_ = resp.GetResult()
			}
		})
		b.Run(fmt.Sprintf("IgnoreReq_%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				resp, err := c.IgnoreReq(context.Background(), &Request{Value: int64(requestValue)})
				if err != nil {
					b.Fatalf("IgnoreReq error: %v", err)
				}
				_ = resp.GetResult()
			}
		})
		// close manager and stop gRPC servers;
		// must be done for each iteration
		closeFn()
		stop()
	}
}
