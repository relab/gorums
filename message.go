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

// MetadataEntry is a key-value pair in a message's metadata.
type MetadataEntry = stream.MetadataEntry

// newResponse returns the response envelope to the request in. It carries the
// request's message ID and method, and either the status of err or the
// marshaled resp. A marshal failure becomes the response status.
func newResponse(in *stream.Message, resp proto.Message, err error) *stream.Message {
	out := stream.Message_builder{
		MessageSeqNo: in.GetMessageSeqNo(), // routes the response to the pending call
		Method:       in.GetMethod(),       // selects the response type on the client
	}.Build()
	if err == nil && resp != nil {
		var payload []byte
		if payload, err = proto.Marshal(resp); err == nil {
			out.SetPayload(payload)
		}
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
