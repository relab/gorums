package gorums

import (
	"github.com/relab/gorums/internal/impl"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
)

// Call represents a lazily dispatched quorum call.
// Register interceptors with [Call.Intercept] before the call is dispatched.
// A Call may be dispatched once.
type Call[Req, Resp proto.Message] = impl.Call[Req, Resp]

// OnewayCall represents a lazily dispatched multicast or unicast call.
// [OnewayCall.Send] and [OnewayCall.Async] each dispatch the call; invoking
// either after the call was dispatched panics. A server handler that must
// keep its one-way calls in FIFO order dispatches them with Send or Async
// before [ServerContext.Release]. To wait for local stream writes to finish,
// it uses Async and calls [OnewayAsync.Wait] after Release.
// A call dispatched after Release may overtake the next request's call.
type OnewayCall[Req proto.Message] = impl.OnewayCall[Req]

// OnewayAsync is the send-completion handle of a one-way call dispatched with
// [OnewayCall.Async]; [OnewayAsync.Wait] returns the send error.
type OnewayAsync = impl.OnewayAsync

// CallContext provides an interceptor with the context and state of a call.
// An interceptor reads the call through the accessors [CallContext.Request],
// [CallContext.Config], [CallContext.Method], [CallContext.Nodes],
// [CallContext.Node], and [CallContext.Size].
type CallContext[Req, Resp proto.Message] = impl.CallContext[Req, Resp]

// ClientInterceptor transforms a call's request or response sequence.
type ClientInterceptor[Req, Resp proto.Message] = impl.ClientInterceptor[Req, Resp]

// MapRequest returns an interceptor that sets the request of each node to
// fn(req, node). Intercept calls fn at once, for each node in configuration
// order. Returning nil for a node skips that node with [ErrSkipNode].
func MapRequest[Req, Resp proto.Message](fn func(Req, *Node) Req) ClientInterceptor[Req, Resp] {
	return impl.MapRequest[Req, Resp](fn)
}

// MapResponse returns an interceptor that transforms each successful response.
func MapResponse[Req, Resp proto.Message](fn func(Resp, *Node) Resp) ClientInterceptor[Req, Resp] {
	return impl.MapResponse[Req](fn)
}

// Responses provides response iteration and aggregation for a quorum call.
type Responses[Resp proto.Message] = impl.Responses[Resp]

// Agreement contains a representative value and the successful node
// responses that established it. Responses are retained in arrival order.
// [Responses.Matching] returns an Agreement.
type Agreement[Resp proto.Message] = impl.Agreement[Resp]

// LevelNotSet is the zero value level used to indicate that no level
// (and thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = impl.LevelNotSet

// Correctable represents the progressive result of a correctable quorum call.
type Correctable[Resp any] = impl.Correctable[Resp]

// ResponseSeq yields the responses from a quorum call.
type ResponseSeq[T proto.Message] = impl.ResponseSeq[T]

// AggregateFunc aggregates the responses of a quorum call into a result of
// type Out, which may differ from the response type. It receives the call's
// single-use response sequence, decides how to handle each node error, and
// decides when to stop. [Responses.Aggregate] runs an AggregateFunc.
type AggregateFunc[Resp proto.Message, Out any] = impl.AggregateFunc[Resp, Out]

// NodeResponse contains a node's response value or error, and the round-trip
// time of the response.
type NodeResponse[T any] = stream.NodeResponse[T]
