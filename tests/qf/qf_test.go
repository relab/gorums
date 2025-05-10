package qf

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const requestValue = 0x1001_1001

type testQSpec struct {
	quorum int
}

func (q testQSpec) SliceUseReqQF(in *Request, replies []*Response) (*Response, bool) {
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

func (q testQSpec) SliceIgnoreReqQF(_ *Request, replies []*Response) (*Response, bool) {
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

func (q testQSpec) SliceWithoutReqQF(replies []*Response) (*Response, bool) {
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

func (q testQSpec) UseReqQF(in *Request, replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	_ = in.GetValue()
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

func (q testQSpec) IgnoreReqQF(_ *Request, replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

func (q testQSpec) WithoutReqQF(replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

func IterUseReq(replies gorums.Iterator[*Response], quorum int, req *Request) (*Response, error) {
	replyCount := 0
	for reply := range replies.IgnoreErrors() {
		msg := reply.Msg
		replyCount++
		if replyCount < quorum {
			continue
		}
		_ = req.GetValue()
		return msg, nil
	}
	return nil, errors.New("UseReq: quorum not found")
}

func IterIgnoreReq(replies gorums.Iterator[*Response], quorum int) (*Response, error) {
	replyCount := 0
	var firstMsg *Response
	for reply := range replies.IgnoreErrors() {
		msg := reply.Msg
		if firstMsg == nil {
			firstMsg = msg
		}
		replyCount++
		if replyCount < quorum {
			continue
		}
		return firstMsg, nil
	}
	return nil, errors.New("IgnoreReq: quorum not found")
}

func BenchmarkQF(b *testing.B) {
	for n := 3; n < 20; n += 2 {
		quorum := n / 2
		mapSize := n << 1
		qspec := &testQSpec{quorum: quorum}
		request := Request_builder{Value: 1}.Build()

		b.Run(fmt.Sprintf("UseReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				replies := make(map[uint32]*Response, mapSize)
				for j := range n {
					replies[uint32(j)] = Response_builder{Result: request.GetValue()}.Build()
					resp, q := qspec.UseReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("IgnoreReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				replies := make(map[uint32]*Response, mapSize)
				for j := range n {
					replies[uint32(j)] = Response_builder{Result: request.GetValue()}.Build()
					resp, q := qspec.IgnoreReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("WithoutReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				replies := make(map[uint32]*Response, mapSize)
				for j := range n {
					replies[uint32(j)] = Response_builder{Result: request.GetValue()}.Build()
					resp, q := qspec.WithoutReqQF(replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})

		// Slice versions

		b.Run(fmt.Sprintf("SliceUseReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				replies := make([]*Response, n)
				for j := range n {
					replies[uint32(j)] = Response_builder{Result: request.GetValue()}.Build()
					resp, q := qspec.SliceUseReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("SliceIgnoreReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				replies := make([]*Response, n)
				for j := range n {
					replies[uint32(j)] = Response_builder{Result: request.GetValue()}.Build()
					resp, q := qspec.SliceIgnoreReqQF(request, replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})
		b.Run(fmt.Sprintf("SliceWithoutReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				replies := make([]*Response, n)
				for j := range n {
					replies[uint32(j)] = Response_builder{Result: request.GetValue()}.Build()
					resp, q := qspec.SliceWithoutReqQF(replies)
					if q {
						_ = resp.GetResult()
					}
				}
			}
		})

		b.Run(fmt.Sprintf("IterUseReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				iterator := func(yield func(gorums.Response[*Response]) bool) {
					for j := range n {
						response := gorums.NewResponse(
							Response_builder{Result: request.GetValue()}.Build(),
							nil,
							uint32(j),
						)
						if !yield(response) {
							return
						}
					}
				}

				resp, _ := IterUseReq(iterator, qspec.quorum, request)
				_ = resp.GetResult()
			}
		})

		b.Run(fmt.Sprintf("IterIgnoreReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				iterator := func(yield func(gorums.Response[*Response]) bool) {
					for j := range n {
						response := gorums.NewResponse(
							Response_builder{Result: request.GetValue()}.Build(),
							nil,
							uint32(j),
						)
						if !yield(response) {
							return
						}
					}
				}

				resp, _ := IterIgnoreReq(iterator, qspec.quorum)
				_ = resp.GetResult()
			}
		})
	}
}

type testSrv struct{}

func (testSrv) UseReq(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{Result: req.GetValue()}.Build(), nil
}

func (testSrv) IgnoreReq(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{Result: req.GetValue()}.Build(), nil
}

func BenchmarkFullStackQF(b *testing.B) {
	for n := 3; n < 20; n += 2 {
		_, stop := gorums.TestSetup(b, n, func(_ int) gorums.ServerIface {
			srv := gorums.NewServer()
			RegisterQuorumFunctionServer(srv, &testSrv{})
			return srv
		})
		mgr := NewManager(
			gorums.WithGrpcDialOptions(
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			),
		)
		c, err := mgr.NewConfiguration(
			gorums.WithNodeList([]string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}), // dummy node list; won't actually be used in test
		)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()

		quorum := n / 2

		b.Run(fmt.Sprintf("UseReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				req := Request_builder{Value: int64(requestValue)}.Build()
				resp, err := IterUseReq(c.UseReq(context.Background(), req), quorum, req)
				if err != nil {
					b.Fatalf("UseReq error: %v", err)
				}
				_ = resp.GetResult()
			}
		})
		b.Run(fmt.Sprintf("IgnoreReq_%d", n), func(b *testing.B) {
			for b.Loop() {
				req := Request_builder{Value: int64(requestValue)}.Build()
				resp, err := IterIgnoreReq(c.IgnoreReq(context.Background(), req), quorum)
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
