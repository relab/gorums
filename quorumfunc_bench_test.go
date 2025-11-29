package gorums

import (
	"context"
	"fmt"
	"testing"

	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// Benchmarks for quorum functions and interceptor chains.
//
// These benchmarks compare:
// - Built-in base quorum functions (MajorityQuorum, ThresholdQuorum, etc.)
// - Custom quorum functions with various iterator patterns
// - Legacy QuorumSpec-style functions via QuorumSpecInterceptor
// - Interceptor chains with different configurations

// -------------------------------------------------------------------------
// Full Stack Benchmarks (real servers)
// -------------------------------------------------------------------------

// BenchmarkQuorumFunction benchmarks the built-in quorum functions with real servers.
func BenchmarkQuorumFunction(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		addrs, stop := TestSetup(b, numNodes, echoServerFn)
		b.Cleanup(stop)

		ctx := context.Background()
		cfgCtx := NewTestConfigContext(b, ctx, addrs)

		b.Run(fmt.Sprintf("MajorityQuorum/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					MajorityQuorum[*pb.StringValue, *pb.StringValue],
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("ThresholdQuorum/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			threshold := numNodes/2 + 1
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					ThresholdQuorum[*pb.StringValue, *pb.StringValue](threshold),
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("FirstResponse/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					FirstResponse[*pb.StringValue, *pb.StringValue],
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("AllResponses/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					AllResponses[*pb.StringValue, *pb.StringValue],
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})
	}
}

// BenchmarkCustomQuorumFunction benchmarks custom quorum functions using different
// iterator patterns (CollectAll, CollectN, etc.)
func BenchmarkCustomQuorumFunction(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		addrs, stop := TestSetup(b, numNodes, echoServerFn)
		b.Cleanup(stop)

		ctx := context.Background()
		cfgCtx := NewTestConfigContext(b, ctx, addrs)

		// Custom QF using CollectAll and then checking quorum
		b.Run(fmt.Sprintf("CollectAllThenCheck/%d", numNodes), func(b *testing.B) {
			qf := func(ctx *ClientCtx[*pb.StringValue, *pb.StringValue]) (*pb.StringValue, error) {
				quorum := ctx.Size()/2 + 1
				replies := ctx.Responses().IgnoreErrors().CollectAll()
				if len(replies) < quorum {
					return nil, QuorumCallError{cause: ErrIncomplete, replies: len(replies)}
				}
				// Return first reply
				for _, r := range replies {
					return r, nil
				}
				return nil, QuorumCallError{cause: ErrIncomplete, replies: 0}
			}
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.String("test"), mock.TestMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Custom QF using CollectN for early termination
		b.Run(fmt.Sprintf("CollectN/%d", numNodes), func(b *testing.B) {
			qf := func(ctx *ClientCtx[*pb.StringValue, *pb.StringValue]) (*pb.StringValue, error) {
				quorum := ctx.Size()/2 + 1
				replies := ctx.Responses().IgnoreErrors().CollectN(quorum)
				if len(replies) < quorum {
					return nil, QuorumCallError{cause: ErrIncomplete, replies: len(replies)}
				}
				// Return first reply
				for _, r := range replies {
					return r, nil
				}
				return nil, QuorumCallError{cause: ErrIncomplete, replies: 0}
			}
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.String("test"), mock.TestMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Custom QF using manual iteration with early return
		b.Run(fmt.Sprintf("ManualIteration/%d", numNodes), func(b *testing.B) {
			qf := func(ctx *ClientCtx[*pb.StringValue, *pb.StringValue]) (*pb.StringValue, error) {
				quorum := ctx.Size()/2 + 1
				var firstResp *pb.StringValue
				var count int
				for result := range ctx.Responses() {
					if result.Err == nil {
						count++
						if firstResp == nil {
							firstResp = result.Value
						}
						if count >= quorum {
							return firstResp, nil
						}
					}
				}
				return nil, QuorumCallError{cause: ErrIncomplete, replies: count}
			}
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.String("test"), mock.TestMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Custom QF using Filter
		b.Run(fmt.Sprintf("WithFilter/%d", numNodes), func(b *testing.B) {
			qf := func(ctx *ClientCtx[*pb.StringValue, *pb.StringValue]) (*pb.StringValue, error) {
				quorum := ctx.Size()/2 + 1
				// Filter responses (in this case, accept all valid responses)
				filtered := ctx.Responses().Filter(func(r NodeResponse[*pb.StringValue]) bool {
					return r.Err == nil && r.Value != nil
				})
				replies := filtered.CollectN(quorum)
				if len(replies) < quorum {
					return nil, QuorumCallError{cause: ErrIncomplete, replies: len(replies)}
				}
				for _, r := range replies {
					return r, nil
				}
				return nil, QuorumCallError{cause: ErrIncomplete, replies: 0}
			}
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.String("test"), mock.TestMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})
	}
}

// BenchmarkQuorumSpecInterceptor benchmarks the legacy QuorumSpec adapter
// compared to using quorum functions directly.
func BenchmarkQuorumSpecInterceptor(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		addrs, stop := TestSetup(b, numNodes, echoServerFn)
		b.Cleanup(stop)

		ctx := context.Background()
		cfgCtx := NewTestConfigContext(b, ctx, addrs)

		// Legacy-style quorum function
		cfgSize := cfgCtx.Configuration().Size()
		legacyQF := func(req *pb.StringValue, replies map[uint32]*pb.StringValue) (*pb.StringValue, bool) {
			quorum := cfgSize/2 + 1
			if len(replies) < quorum {
				return nil, false
			}
			_ = req.GetValue() // Use request
			for _, r := range replies {
				return r, true
			}
			return nil, false
		}

		// Using QuorumSpecInterceptor adapter
		b.Run(fmt.Sprintf("ViaInterceptor/%d", numNodes), func(b *testing.B) {
			interceptor := QuorumSpecInterceptor(legacyQF)
			qf := interceptor(nil)
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.String("test"), mock.TestMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Using QuorumSpecFunc directly
		b.Run(fmt.Sprintf("ViaFunc/%d", numNodes), func(b *testing.B) {
			qf := QuorumSpecFunc(legacyQF)
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.String("test"), mock.TestMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Direct iterator-based equivalent
		b.Run(fmt.Sprintf("DirectIterator/%d", numNodes), func(b *testing.B) {
			qf := func(ctx *ClientCtx[*pb.StringValue, *pb.StringValue]) (*pb.StringValue, error) {
				quorum := ctx.Size()/2 + 1
				replies := ctx.Responses().IgnoreErrors().CollectAll()
				if len(replies) < quorum {
					return nil, QuorumCallError{cause: ErrIncomplete, replies: len(replies)}
				}
				_ = ctx.Request().GetValue() // Use request
				for _, r := range replies {
					return r, nil
				}
				return nil, QuorumCallError{cause: ErrIncomplete, replies: 0}
			}
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.String("test"), mock.TestMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})
	}
}

// BenchmarkInterceptorChain benchmarks the overhead of chaining interceptors.
func BenchmarkInterceptorChain(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7} {
		addrs, stop := TestSetup(b, numNodes, echoServerFn)
		b.Cleanup(stop)

		ctx := context.Background()
		cfgCtx := NewTestConfigContext(b, ctx, addrs)

		// No-op interceptor for measuring chain overhead
		noopInterceptor := func(next QuorumFunc[*pb.StringValue, *pb.StringValue, *pb.StringValue]) QuorumFunc[*pb.StringValue, *pb.StringValue, *pb.StringValue] {
			return func(ctx *ClientCtx[*pb.StringValue, *pb.StringValue]) (*pb.StringValue, error) {
				return next(ctx)
			}
		}

		// Baseline: no interceptors
		b.Run(fmt.Sprintf("NoInterceptors/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					MajorityQuorum[*pb.StringValue, *pb.StringValue],
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// One interceptor
		b.Run(fmt.Sprintf("OneInterceptor/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					Chain(
						MajorityQuorum[*pb.StringValue, *pb.StringValue],
						noopInterceptor,
					),
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Three interceptors
		b.Run(fmt.Sprintf("ThreeInterceptors/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					Chain(
						MajorityQuorum[*pb.StringValue, *pb.StringValue],
						noopInterceptor,
						noopInterceptor,
						noopInterceptor,
					),
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Five interceptors
		b.Run(fmt.Sprintf("FiveInterceptors/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					Chain(
						MajorityQuorum[*pb.StringValue, *pb.StringValue],
						noopInterceptor,
						noopInterceptor,
						noopInterceptor,
						noopInterceptor,
						noopInterceptor,
					),
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// With Map interceptor (transforms request and response)
		b.Run(fmt.Sprintf("WithMapInterceptor/%d", numNodes), func(b *testing.B) {
			mapInterceptor := Map[*pb.StringValue, *pb.StringValue, *pb.StringValue](
				func(req *pb.StringValue, node *RawNode) *pb.StringValue {
					// Transform request per node
					return pb.String(req.GetValue() + fmt.Sprintf("-%d", node.ID()))
				},
				func(resp *pb.StringValue, node *RawNode) *pb.StringValue {
					// Transform response
					return resp
				},
			)
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
					Chain(
						MajorityQuorum[*pb.StringValue, *pb.StringValue],
						mapInterceptor,
					),
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})
	}
}

// BenchmarkAggregation benchmarks different response aggregation strategies.
func BenchmarkAggregation(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		// Use GetValue method that returns integers for summing
		addrs, stop := TestSetup(b, numNodes, nil)
		b.Cleanup(stop)

		ctx := context.Background()
		cfgCtx := NewTestConfigContext(b, ctx, addrs)

		// Sum all responses
		b.Run(fmt.Sprintf("SumResponses/%d", numNodes), func(b *testing.B) {
			qf := func(ctx *ClientCtx[*pb.Int32Value, *pb.Int32Value]) (*pb.Int32Value, error) {
				var sum int32
				for result := range ctx.Responses().IgnoreErrors() {
					sum += result.Value.GetValue()
				}
				return pb.Int32(sum), nil
			}
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.Int32(0), mock.GetValueMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Find max response
		b.Run(fmt.Sprintf("MaxResponse/%d", numNodes), func(b *testing.B) {
			qf := func(ctx *ClientCtx[*pb.Int32Value, *pb.Int32Value]) (*pb.Int32Value, error) {
				var maxVal int32
				for result := range ctx.Responses().IgnoreErrors() {
					if result.Value.GetValue() > maxVal {
						maxVal = result.Value.GetValue()
					}
				}
				return pb.Int32(maxVal), nil
			}
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(cfgCtx, pb.Int32(0), mock.GetValueMethod, qf)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		// Collect all into map
		b.Run(fmt.Sprintf("CollectAllMap/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor(
					cfgCtx,
					pb.Int32(0),
					mock.GetValueMethod,
					CollectAllResponses[*pb.Int32Value, *pb.Int32Value],
				)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = len(resp)
			}
		})
	}
}
