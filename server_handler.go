package gorums

import (
	"context"
	"slices"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Message wraps a wire-level [stream.Message] with its deserialized proto payload.
// It is used by both server and client handler chains to carry the application-level
// message alongside the stream-level envelope.
type Message struct {
	Proto proto.Message
	*stream.Message
}

// MetadataEntry is a type alias for [stream.MetadataEntry].
type MetadataEntry = stream.MetadataEntry

type (
	// Handler processes a request and returns a response.
	Handler func(ServerContext, *Message) (*Message, error)
	// ServerInterceptor intercepts and may modify incoming requests and outgoing responses.
	// It receives a ServerContext, the incoming Message, and a Handler representing
	// the next element in the chain. It returns a Message and an error.
	ServerInterceptor func(ServerContext, *Message, Handler) (*Message, error)
)

// ServerContext is the context a Gorums server passes to a handler.
// Requests on one stream run one at a time until the handler calls
// [ServerContext.Release] or returns.
type ServerContext struct {
	context.Context
	release func()
	send    func(*stream.Message)
	srv     *Server
}

// Release lets the next request on this handler's stream start, concurrently
// with this handler. Replies to calls the handler makes arrive before and after
// Release, so the handler may call the peer that sent the request. Before
// Release, such a reply arrives only while the stream's dispatch queue has room
// (see [WithBufferSizes]); call Release first if the peer may fill that queue
// while the call is in flight. Call Release once the handler is done with state
// that requests must access one at a time. It is safe to call Release multiple
// times.
func (ctx *ServerContext) Release() {
	if ctx.release != nil {
		ctx.release()
	}
}

// SendMessage sends the given message to the client.
// If marshaling fails, the error is encoded into the response envelope
// and sent to the client; the stream is not closed.
//
// This function should only be used by generated code.
func (ctx *ServerContext) SendMessage(out *Message) {
	// If Proto is set, marshal it to payload before sending.
	if out.Proto != nil && len(out.GetPayload()) == 0 {
		payload, err := proto.Marshal(out.Proto)
		if err == nil {
			out.SetPayload(payload)
		} else {
			// Encode the marshal error into the response envelope; don't close the stream.
			out = messageWithError(nil, out, err)
		}
	}
	if ctx.send != nil {
		ctx.send(out.Message)
	}
}

// PeerConfig returns the [Config] of the peers configured with [WithPeers],
// or nil if [WithPeers] was not used. It is the same configuration as
// [Server.PeerConfig], so a handler can fan out calls to the server's peers.
// Call [ServerContext.Release] before invoking calls on it, so that the next
// request on this stream can start while the handler waits for responses.
func (ctx *ServerContext) PeerConfig() Config {
	if ctx.srv == nil {
		return nil
	}
	return ctx.srv.PeerConfig()
}

// ConnectedClients returns a [Config] of the clients currently connected to
// this server that can receive back-channel calls. It is the same
// configuration as [Server.ConnectedClients]. An empty (non-nil)
// configuration is returned when no clients are connected.
func (ctx *ServerContext) ConnectedClients() Config {
	if ctx.srv == nil {
		return nil
	}
	return ctx.srv.ConnectedClients()
}

// NewResponseMessage creates a new response envelope based on the provided proto
// message. The response includes the message ID and method from the request
// to facilitate routing the response back to the caller on the client side.
// The payload, error status, and metadata entries are left empty;
// [ServerContext.SendMessage] marshals the payload. This function is safe for
// concurrent use.
//
// This function should only be used in generated code.
func NewResponseMessage(in *Message, resp proto.Message) *Message {
	if in == nil {
		return nil
	}
	// Create a new Message to avoid race conditions when the sender
	// goroutine marshals while the handler creates the next response.
	msgBuilder := stream.Message_builder{
		MessageSeqNo: in.GetMessageSeqNo(), // needed in RouteResponse to lookup the response channel
		Method:       in.GetMethod(),       // needed in UnmarshalResponse to look up the response type in the proto registry
		// Payload is left empty; SendMessage will marshal resp into the payload when sending the message
		// Status is left empty; it can be set by messageWithError if needed
	}
	return &Message{
		Proto:   resp,
		Message: msgBuilder.Build(),
	}
}

// messageWithError ensures a response envelope exists and sets the error status.
// If out is nil, a new response is created based on the in request envelope;
// otherwise, out is modified in place. This is used by the server to send error
// responses back to the client.
func messageWithError(in, out *Message, err error) *Message {
	if out == nil {
		out = NewResponseMessage(in, nil)
	}
	if err != nil {
		errStatus, ok := status.FromError(err)
		if !ok {
			errStatus = status.New(codes.Unknown, err.Error())
		}
		out.SetStatus(errStatus.Proto())
	}
	return out
}

// AsProto returns the envelope's already-decoded proto message as type T.
// If the envelope is nil, or the underlying message cannot be asserted to T,
// the zero value of T is returned.
func AsProto[T proto.Message](msg *Message) T {
	var zero T
	if msg == nil || msg.Proto == nil {
		return zero
	}
	if req, ok := msg.Proto.(T); ok {
		return req
	}
	return zero
}

// chainInterceptors composes the provided interceptors around the final Handler and
// returns a Handler that executes the chain. The execution order is the same as the
// order of the interceptors in the slice: the first element is executed first, and
// the last element calls the final handler (the server method).
func chainInterceptors(final Handler, interceptors ...ServerInterceptor) Handler {
	if len(interceptors) == 0 {
		return final
	}
	handler := final
	for _, curr := range slices.Backward(interceptors) {
		next := handler
		handler = func(ctx ServerContext, in *Message) (*Message, error) {
			return curr(ctx, in, next)
		}
	}
	return handler
}
