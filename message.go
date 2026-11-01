package gorums

import (
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

// newResponseMessage creates a response envelope for the given request and
// proto message. The sender marshals its payload before transmission.
func newResponseMessage(in *Message, resp proto.Message) *Message {
	if in == nil {
		return nil
	}
	return &Message{
		Proto:   resp,
		Message: stream.NewResponseEnvelope(in.Message),
	}
}

// messageWithError ensures a response envelope exists and sets the error status.
// If out is nil, a new response is created based on the in request envelope;
// otherwise, out is modified in place. This is used by the server to send error
// responses back to the client.
func messageWithError(in, out *Message, err error) *Message {
	if out == nil {
		out = newResponseMessage(in, nil)
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
