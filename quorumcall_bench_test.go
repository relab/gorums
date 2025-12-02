package gorums

import (
	"fmt"
	"testing"

	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// Benchmarks for terminal methods on Responses.
//
// These benchmarks compare the built-in terminal methods:
// - First() - returns first successful response
// - Majority() - waits for simple majority
// - All() - waits for all responses
// - Threshold(n) - waits for n responses

// BenchmarkTerminalMethods benchmarks the built-in terminal methods with real servers.
func BenchmarkTerminalMethods(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		cfg := SetupConfiguration(b, numNodes, echoServerFn)
		cfgCtx := WithConfigContext(b.Context(), cfg)

		b.Run(fmt.Sprintf("Majority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).Majority()
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("Threshold/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			threshold := numNodes/2 + 1
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).Threshold(threshold)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("First/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).First()
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("All/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).All()
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})
	}
}

// BenchmarkIteratorPatterns benchmarks custom aggregation using different iterator patterns.
func BenchmarkIteratorPatterns(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		cfg := SetupConfiguration(b, numNodes, echoServerFn)
		cfgCtx := WithConfigContext(b.Context(), cfg)

		// Using CollectAll and then checking quorum
		b.Run(fmt.Sprintf("CollectAllThenCheck/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				)
				replies := responses.CollectAll()
				if len(replies) < quorum {
					b.Fatalf("not enough replies: %d < %d", len(replies), quorum)
				}
				// Return first reply
				for _, r := range replies {
					_ = r.GetValue()
					break
				}
			}
		})

		// Using CollectN for early termination
		b.Run(fmt.Sprintf("CollectN/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				)
				replies := responses.CollectN(quorum)
				if len(replies) < quorum {
					b.Fatalf("not enough replies: %d < %d", len(replies), quorum)
				}
				// Return first reply
				for _, r := range replies {
					_ = r.GetValue()
					break
				}
			}
		})

		// Using manual iteration with early return
		b.Run(fmt.Sprintf("ManualIteration/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				)
				var firstResp *pb.StringValue
				var count int
				for result := range responses.Seq() {
					if result.Err == nil {
						count++
						if firstResp == nil {
							firstResp = result.Value
						}
						if count >= quorum {
							break
						}
					}
				}
				if count < quorum {
					b.Fatalf("not enough responses: %d < %d", count, quorum)
				}
				_ = firstResp.GetValue()
			}
		})
	}
}
