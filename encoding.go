package gorums

import (
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
)

// MetadataEntry is a type alias for stream.MetadataEntry.
type MetadataEntry = stream.MetadataEntry

// MetadataEntry_builder is a type alias for stream.MetadataEntry_builder.
type MetadataEntry_builder = stream.MetadataEntry_builder

// NewResponseMessage creates a new response message based on the provided proto
// message. The response message includes the message ID and method from the request
// message to facilitate routing the response back to the caller on the client side.
//
// This function should only be used in generated code.
func NewResponseMessage(in *Message, resp proto.Message) *Message {
	return stream.NewResponseMessage(in, resp)
}

// AsProto returns the message's already-decoded proto message as type T.
// If the message is nil, or the underlying message cannot be asserted to T,
// the zero value of T is returned.
func AsProto[T proto.Message](msg *Message) T {
	return stream.AsProto[T](msg)
}
