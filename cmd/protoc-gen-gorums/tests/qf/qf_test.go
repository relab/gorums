package qf

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

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

// TODO(meling) consider making these things generally available, at least for testing, perhaps putting some of these things in an internal package.

type portSupplier struct {
	p int
	sync.Mutex
}

func (p *portSupplier) get() int {
	p.Lock()
	_p := p.p
	p.p++
	p.Unlock()
	return _p
}

var supplier = portSupplier{p: 22332}

func getListener() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", supplier.get()))
}

type testSrv struct {
	dummy int64
}

func (s testSrv) UseReq(_ context.Context, req *Request) (*Response, error) {
	return &Response{Result: req.GetValue()}, nil
}

func (s testSrv) IgnoreReq(_ context.Context, req *Request) (*Response, error) {
	return &Response{Result: req.GetValue()}, nil
}

func (s testSrv) WithoutReq(_ context.Context, req *Request) (*Response, error) {
	return &Response{Result: req.GetValue()}, nil
}

func setup(b *testing.B, numServers int) ([]*grpc.Server, *Manager, *Configuration) {
	quorum := numServers / 2
	servers := make([]*grpc.Server, numServers)
	addrs := make([]string, numServers)
	for i := 0; i < numServers; i++ {
		srv := grpc.NewServer()
		RegisterQuorumFunctionServer(srv, &testSrv{})
		lis, err := getListener()
		if err != nil {
			b.Fatalf("Failed to listen on port: %v", err)
		}
		addrs[i] = lis.Addr().String()
		servers[i] = srv
		go srv.Serve(lis)
	}

	// client setup
	man, err := NewManager(addrs,
		WithGrpcDialOptions(grpc.WithInsecure()),
		WithDialTimeout(10*time.Second),
	)
	if err != nil {
		b.Fatalf("Failed to create manager: %v", err)
	}
	c, err := man.NewConfiguration(man.NodeIDs(), &testQSpec{quorum: quorum})
	if err != nil {
		b.Fatalf("Failed to create configuration: %v", err)
	}
	return servers, man, c
}

func BenchmarkUseReq(b *testing.B) {
	servers, man, c := setup(b, 5)

	b.ReportAllocs()
	b.ResetTimer()

	// begin benchmarking
	for i := 0; i < b.N; i++ {
		resp, err := c.UseReq(context.Background(), &Request{Value: int64(requestValue)})
		if err != nil {
			b.Fatalf("UseReq error: %v", err)
		}
		_ = resp.GetResult()
	}

	man.Close()
	for _, srv := range servers {
		srv.Stop()
	}
}

func BenchmarkIgnoreReq(b *testing.B) {
	servers, man, c := setup(b, 5)

	b.ReportAllocs()
	b.ResetTimer()

	// begin benchmarking
	for i := 0; i < b.N; i++ {
		resp, err := c.IgnoreReq(context.Background(), &Request{Value: int64(requestValue)})
		if err != nil {
			b.Fatalf("IgnoreReq error: %v", err)
		}
		_ = resp.GetResult()
	}

	man.Close()
	for _, srv := range servers {
		srv.Stop()
	}
}

func BenchmarkWithoutReq(b *testing.B) {
	servers, man, c := setup(b, 5)

	b.ReportAllocs()
	b.ResetTimer()

	// begin benchmarking
	for i := 0; i < b.N; i++ {
		resp, err := c.WithoutReq(context.Background(), &Request{Value: int64(requestValue)})
		if err != nil {
			b.Fatalf("WithoutReq error: %v", err)
		}
		_ = resp.GetResult()
	}

	man.Close()
	for _, srv := range servers {
		srv.Stop()
	}
}
