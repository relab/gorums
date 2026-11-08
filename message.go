package gorums

import (
	"github.com/relab/gorums/internal/stream"
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
