package stream

import (
	"errors"

	"google.golang.org/protobuf/proto"
)

// NodeResponse wraps a response value from node ID, and an error if any.
type NodeResponse[T any] struct {
	NodeID uint32
	Value  T
	Err    error
}

// response is a type alias for NodeResponse[proto.Message] to avoid long type names.
type response = NodeResponse[proto.Message]

// ErrTypeMismatch is returned when a response cannot be cast to the expected type.
var ErrTypeMismatch = errors.New("response type mismatch")
