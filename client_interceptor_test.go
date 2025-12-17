package gorums_test

import (
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// LoggingInterceptor is a custom interceptor that logs each response.
func LoggingInterceptor[Req, Resp proto.Message](
	ctx *gorums.ClientCtx[Req, Resp],
	next gorums.ResponseSeq[Resp],
) gorums.ResponseSeq[Resp] {
	_ = ctx.Method() // Access method name (could be used for logging)
	return func(yield func(gorums.NodeResponse[Resp]) bool) {
		for resp := range next {
			// In a real interceptor, you would log here
			if !yield(resp) {
				return
			}
		}
	}
}

// FilterInterceptor returns an interceptor that filters responses based on a predicate.
func FilterInterceptor[Req, Resp proto.Message](
	keep func(resp gorums.NodeResponse[Resp]) bool,
) gorums.QuorumInterceptor[Req, Resp] {
	return func(ctx *gorums.ClientCtx[Req, Resp], next gorums.ResponseSeq[Resp]) gorums.ResponseSeq[Resp] {
		_ = ctx.Method() // Access method name (could be used for filtering)
		return func(yield func(gorums.NodeResponse[Resp]) bool) {
			for resp := range next {
				if keep(resp) {
					if !yield(resp) {
						return
					}
				}
			}
		}
	}
}

// CountingInterceptor counts the number of responses passing through.
func CountingInterceptor[Req, Resp proto.Message](
	counter *int,
) gorums.QuorumInterceptor[Req, Resp] {
	return func(_ *gorums.ClientCtx[Req, Resp], next gorums.ResponseSeq[Resp]) gorums.ResponseSeq[Resp] {
		return func(yield func(gorums.NodeResponse[Resp]) bool) {
			for resp := range next {
				*counter++
				if !yield(resp) {
					return
				}
			}
		}
	}
}

// TestCustomLoggingInterceptor verifies that custom interceptors can be
// created and used from an external package.
func TestCustomLoggingInterceptor(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, gorums.EchoServerFn)
	ctx := gorums.TestContext(t, 2*time.Second)

	// Use the custom logging interceptor from this external package
	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		config.Context(ctx),
		pb.String("test"),
		mock.TestMethod,
		gorums.Interceptors(LoggingInterceptor[*pb.StringValue, *pb.StringValue]),
	)

	result, err := responses.Majority()
	if err != nil {
		t.Fatalf("QuorumCall failed: %v", err)
	}
	if result.GetValue() != "echo: test" {
		t.Errorf("Expected 'echo: test', got %q", result.GetValue())
	}
}

// TestCustomFilterInterceptor verifies that filter interceptors work correctly.
func TestCustomFilterInterceptor(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, gorums.EchoServerFn)
	ctx := gorums.TestContext(t, 2*time.Second)

	// Use a filter interceptor that only keeps responses from node 1
	// (In practice, this would filter based on response content)
	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		config.Context(ctx),
		pb.String("test"),
		mock.TestMethod,
		gorums.Interceptors(FilterInterceptor[*pb.StringValue](
			func(resp gorums.NodeResponse[*pb.StringValue]) bool {
				return resp.Err == nil // Only keep successful responses
			},
		)),
	)

	result, err := responses.First()
	if err != nil {
		t.Fatalf("QuorumCall failed: %v", err)
	}
	if result.GetValue() != "echo: test" {
		t.Errorf("Expected 'echo: test', got %q", result.GetValue())
	}
}

// TestInterceptorChaining verifies that multiple custom interceptors can be chained.
func TestInterceptorChaining(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, gorums.EchoServerFn)
	ctx := gorums.TestContext(t, 2*time.Second)

	var count int

	// Chain multiple custom interceptors
	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		config.Context(ctx),
		pb.String("test"),
		mock.TestMethod,
		gorums.Interceptors(
			LoggingInterceptor[*pb.StringValue, *pb.StringValue],
			CountingInterceptor[*pb.StringValue, *pb.StringValue](&count),
		),
	)

	result, err := responses.Majority()
	if err != nil {
		t.Fatalf("QuorumCall failed: %v", err)
	}
	if result.GetValue() != "echo: test" {
		t.Errorf("Expected 'echo: test', got %q", result.GetValue())
	}

	// Majority requires 2 responses out of 3, so count should be at least 2
	if count < 2 {
		t.Errorf("Expected at least 2 responses counted, got %d", count)
	}
}

// TestCustomInterceptorWithMapRequest verifies custom interceptors work with built-in ones.
func TestCustomInterceptorWithMapRequest(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, gorums.EchoServerFn)
	ctx := gorums.TestContext(t, 2*time.Second)

	var count int

	// Mix custom interceptor with built-in MapRequest
	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		config.Context(ctx),
		pb.String("test"),
		mock.TestMethod,
		gorums.Interceptors(
			// Custom: count responses
			CountingInterceptor[*pb.StringValue, *pb.StringValue](&count),
			// Built-in: transform request (identity transform for this test)
			gorums.MapRequest[*pb.StringValue, *pb.StringValue](
				func(req *pb.StringValue, node *gorums.Node) *pb.StringValue {
					return req
				},
			),
		),
	)

	result, err := responses.All()
	if err != nil {
		t.Fatalf("QuorumCall failed: %v", err)
	}
	if result.GetValue() != "echo: test" {
		t.Errorf("Expected 'echo: test', got %q", result.GetValue())
	}

	// All requires 3 responses
	if count != 3 {
		t.Errorf("Expected 3 responses counted, got %d", count)
	}
}
