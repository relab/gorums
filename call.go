package gorums

import (
	"github.com/relab/gorums/internal/impl"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
)

// Call represents a lazily dispatched quorum call.
// Register interceptors with [Call.Intercept] before consuming its responses.
// A Call may be consumed once.
type Call[Req, Resp proto.Message] = impl.Call[Req, Resp]

// OnewayCall represents a lazily dispatched multicast or unicast call.
// [OnewayCall.Send] and [OnewayCall.Async] each consume the call; invoking
// either after the call has been consumed panics. A handler that dispatches a
// one-way call back to its callers should call [ServerContext.Release] first.
type OnewayCall[Req proto.Message] = impl.OnewayCall[Req]

// OnewayAsync is the send-completion handle of a one-way call dispatched with
// [OnewayCall.Async]; [OnewayAsync.Wait] reports the send error.
type OnewayAsync = impl.OnewayAsync

// CallContext provides an interceptor with the context and state of a call.
type CallContext[Req, Resp proto.Message] = impl.CallContext[Req, Resp]

// ClientInterceptor transforms a call's request or response sequence.
type ClientInterceptor[Req, Resp proto.Message] = impl.ClientInterceptor[Req, Resp]

// MapRequest returns an interceptor that transforms the request for each node.
// Returning nil or an invalid message skips the node with [ErrSkipNode].
func MapRequest[Req, Resp proto.Message](fn func(Req, *Node) Req) ClientInterceptor[Req, Resp] {
	return impl.MapRequest[Req, Resp](fn)
}

// MapResponse returns an interceptor that transforms each successful response.
func MapResponse[Req, Resp proto.Message](fn func(Resp, *Node) Resp) ClientInterceptor[Req, Resp] {
	return impl.MapResponse[Req](fn)
}

// Responses provides response iteration and aggregation for a quorum call.
type Responses[Resp proto.Message] = impl.Responses[Resp]

// Async represents the eventual result of an asynchronous quorum call.
type Async[Resp any] = impl.Async[Resp]

// LevelNotSet is the zero value level used to indicate that no level
// (and thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = impl.LevelNotSet

// Correctable represents the progressive result of a correctable quorum call.
type Correctable[Resp any] = impl.Correctable[Resp]

// ResponseSeq yields the responses from a quorum call.
type ResponseSeq[T proto.Message] = impl.ResponseSeq[T]

// NodeResponse contains a node's response value or error.
type NodeResponse[T any] = stream.NodeResponse[T]
