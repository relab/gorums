package gorumsimpl

import (
	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
)

// NewResponseMessage returns a response envelope for generated unary handlers.
// The server marshals its payload before sending it.
func NewResponseMessage(in *gorums.Message, resp proto.Message) *gorums.Message {
	if in == nil {
		return nil
	}
	return &gorums.Message{
		Proto:   resp,
		Message: stream.NewResponseEnvelope(in.Message),
	}
}
