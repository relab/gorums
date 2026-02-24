package stream

import (
	"context"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Envelope wraps a wire-level [Message] with its deserialized proto payload.
// It is used by both server and client handler chains to carry the application-level
// message alongside the stream-level envelope.
type Envelope struct {
	Msg proto.Message
	*Message
}

type (
	// Handler processes a request and returns a response.
	Handler func(ServerCtx, *Envelope) (*Envelope, error)
	// Interceptor intercepts and may modify incoming requests and outgoing responses.
	// It receives a ServerCtx, the incoming Envelope, and a Handler representing
	// the next element in the chain. It returns an Envelope and an error.
	Interceptor func(ServerCtx, *Envelope, Handler) (*Envelope, error)
)

// ServerCtx is a context that is passed from the Gorums server to the handler.
// It allows the handler to release its lock on the server, allowing the next
// request to be processed. This happens automatically when the handler returns.
type ServerCtx struct {
	context.Context
	once *sync.Once // must be a pointer to avoid passing ctx by value
	mut  *sync.Mutex
	c    chan<- *Message
}

// NewServerCtx creates a new ServerCtx with the given context, mutex and response channel.
func NewServerCtx(ctx context.Context, mut *sync.Mutex, c chan<- *Message) ServerCtx {
	return ServerCtx{
		Context: ctx,
		once:    new(sync.Once),
		mut:     mut,
		c:       c,
	}
}

// Release releases this handler's lock on the server, which allows the next request
// to be processed concurrently. Use Release only when the handler no longer needs
// exclusive access to the server's state. It is safe to call Release multiple times.
func (ctx *ServerCtx) Release() {
	ctx.once.Do(ctx.mut.Unlock)
}

// SendMessage attempts to send the given message to the client.
// This may fail if the stream was closed or the stream context got canceled.
//
// This function should only be used by generated code.
func (ctx *ServerCtx) SendMessage(out *Envelope) error {
	// If Msg is set, marshal it to payload before sending.
	if out.Msg != nil && len(out.GetPayload()) == 0 {
		payload, err := proto.Marshal(out.Msg)
		if err == nil {
			out.SetPayload(payload)
		}
		// Return an error to the client if marshaling failed on the server side; don't close the stream.
		out = MessageWithError(nil, out, err)
	}
	select {
	case ctx.c <- out.Message:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// NewResponseMessage creates a new response envelope based on the provided proto
// message. The response includes the message ID and method from the request
// to facilitate routing the response back to the caller on the client side.
// The payload, error status, and metadata entries are left empty; the error status
// of the response can be set using [MessageWithError], and the payload will
// be marshaled by [ServerCtx.SendMessage]. This function is safe for concurrent use.
//
// This function should only be used in generated code.
func NewResponseMessage(in *Envelope, resp proto.Message) *Envelope {
	if in == nil {
		return nil
	}
	// Create a new Message to avoid race conditions when the sender
	// goroutine marshals while the handler creates the next response.
	msgBuilder := Message_builder{
		MessageSeqNo: in.GetMessageSeqNo(), // needed in RouteResponse to lookup the response channel
		Method:       in.GetMethod(),       // needed in UnmarshalResponse to look up the response type in the proto registry
		// Payload is left empty; SendMessage will marshal resp into the payload when sending the message
		// Status is left empty; it can be set by MessageWithError if needed
	}
	return &Envelope{
		Msg:     resp,
		Message: msgBuilder.Build(),
	}
}

// MessageWithError ensures a response envelope exists and sets the error status.
// If out is nil, a new response is created based on the in request envelope;
// otherwise, out is modified in place. This is used by the server to send error
// responses back to the client.
func MessageWithError(in, out *Envelope, err error) *Envelope {
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
func AsProto[T proto.Message](msg *Envelope) T {
	var zero T
	if msg == nil || msg.Msg == nil {
		return zero
	}
	if req, ok := msg.Msg.(T); ok {
		return req
	}
	return zero
}

// ChainInterceptors composes the provided interceptors around the final Handler and
// returns a Handler that executes the chain. The execution order is the same as the
// order of the interceptors in the slice: the first element is executed first, and
// the last element calls the final handler (the server method).
func ChainInterceptors(final Handler, interceptors ...Interceptor) Handler {
	if len(interceptors) == 0 {
		return final
	}
	handler := final
	for i := len(interceptors) - 1; i >= 0; i-- {
		curr := interceptors[i]
		next := handler
		handler = func(ctx ServerCtx, in *Envelope) (*Envelope, error) {
			return curr(ctx, in, next)
		}
	}
	return handler
}
