package gorums

import (
	"context"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Message wraps a wire-level [stream.Message] with its deserialized proto payload.
// It is used by both server and client handler chains to carry the application-level
// message alongside the stream-level envelope.
type Message struct {
	Msg proto.Message
	*stream.Message
}

// MetadataEntry is a type alias for [stream.MetadataEntry].
type MetadataEntry = stream.MetadataEntry

// MetadataEntry_builder is a type alias for [stream.MetadataEntry_builder].
type MetadataEntry_builder = stream.MetadataEntry_builder

type (
	// Handler processes a request and returns a response.
	Handler func(ServerContext, *Message) (*Message, error)
	// Interceptor intercepts and may modify incoming requests and outgoing responses.
	// It receives a ServerContext, the incoming Message, and a Handler representing
	// the next element in the chain. It returns a Message and an error.
	Interceptor func(ServerContext, *Message, Handler) (*Message, error)
)

// ServerContext is a context that is passed from the Gorums server to the handler.
// It allows the handler to release its lock on the server, allowing the next
// request to be processed. This happens automatically when the handler returns.
type ServerContext struct {
	context.Context
	release func()
	send    func(*stream.Message)
	srv     *Server
}

// Release releases this handler's lock on the server, which allows the next request
// to be processed concurrently. Use Release only when the handler no longer needs
// exclusive access to the server's state. It is safe to call Release multiple times.
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
	// If Msg is set, marshal it to payload before sending.
	if out.Msg != nil && len(out.GetPayload()) == 0 {
		payload, err := proto.Marshal(out.Msg)
		if err == nil {
			out.SetPayload(payload)
		} else {
			// Encode the marshal error into the response envelope; don't close the stream.
			out = MessageWithError(nil, out, err)
		}
	}
	if ctx.send != nil {
		ctx.send(out.Message)
	}
}

// PeerConfig returns the [Config] of the peers the server was configured
// with via [WithPeers], or nil if it was not used. It is the full peer set, not
// the currently reachable subset, so quorum sizes derived from it inside a
// handler do not shift as peers connect and disconnect. Use
// [ServerContext.ConnectedPeers] to observe reachability.
func (ctx *ServerContext) PeerConfig() Config {
	if ctx.srv == nil {
		return nil
	}
	return ctx.srv.PeerConfig()
}

// ConnectedPeers returns the currently reachable subset of
// [ServerContext.PeerConfig]; see [Server.ConnectedPeers].
func (ctx *ServerContext) ConnectedPeers() Config {
	if ctx.srv == nil {
		return nil
	}
	return ctx.srv.ConnectedPeers()
}

// ConnectedClients returns a [Config] of all connected clients capable of
// receiving reverse-direction calls from the server.
// An empty (non-nil) Config is returned if no client peers are connected.
// The returned slice is replaced atomically on each connect/disconnect;
// thus, retaining a reference to an old configuration is safe.
func (ctx *ServerContext) ConnectedClients() Config {
	if ctx.srv == nil {
		return nil
	}
	return ctx.srv.ConnectedClients()
}

// NewResponseMessage creates a new response envelope based on the provided proto
// message. The response includes the message ID and method from the request
// to facilitate routing the response back to the caller on the client side.
// The payload, error status, and metadata entries are left empty; the error status
// of the response can be set using [MessageWithError], and the payload will
// be marshaled by [ServerContext.SendMessage]. This function is safe for concurrent use.
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
		// Status is left empty; it can be set by MessageWithError if needed
	}
	return &Message{
		Msg:     resp,
		Message: msgBuilder.Build(),
	}
}

// MessageWithError ensures a response envelope exists and sets the error status.
// If out is nil, a new response is created based on the in request envelope;
// otherwise, out is modified in place. This is used by the server to send error
// responses back to the client.
func MessageWithError(in, out *Message, err error) *Message {
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
	if msg == nil || msg.Msg == nil {
		return zero
	}
	if req, ok := msg.Msg.(T); ok {
		return req
	}
	return zero
}

// chainInterceptors composes the provided interceptors around the final Handler and
// returns a Handler that executes the chain. The execution order is the same as the
// order of the interceptors in the slice: the first element is executed first, and
// the last element calls the final handler (the server method).
func chainInterceptors(final Handler, interceptors ...Interceptor) Handler {
	if len(interceptors) == 0 {
		return final
	}
	handler := final
	for i := len(interceptors) - 1; i >= 0; i-- {
		curr := interceptors[i]
		next := handler
		handler = func(ctx ServerContext, in *Message) (*Message, error) {
			return curr(ctx, in, next)
		}
	}
	return handler
}
