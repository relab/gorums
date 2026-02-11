package gorums

import (
	"fmt"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Message encapsulates the stream.Message and the actual proto.Message.
type Message struct {
	Msg proto.Message
	*stream.Message
}

// MetadataEntry is a type alias for stream.MetadataEntry.
type MetadataEntry = stream.MetadataEntry

// MetadataEntry_builder is a type alias for stream.MetadataEntry_builder.
type MetadataEntry_builder = stream.MetadataEntry_builder

// NewResponseMessage creates a new response message based on the provided proto
// message. The response message includes the message ID and method from the request
// message to facilitate routing the response back to the caller on the client side.
// The payload, error status, and metadata entries are left empty; the error status
// of the response message can be set using [messageWithError], and the payload will
// be marshaled by [ServerCtx.SendMessage]. This function is safe for concurrent use.
//
// This function should only be used in generated code.
func NewResponseMessage(in *Message, resp proto.Message) *Message {
	if in == nil {
		return nil
	}
	// Create a new stream.Message to avoid race conditions when the sender
	// goroutine marshals while the handler creates the next response.
	// This can happen for stream-based quorum calls where the handler can
	// call SendMessage multiple times before returning.
	msgBuilder := stream.Message_builder{
		MessageSeqNo: in.GetMessageSeqNo(), // needed in channel.routeResponse to lookup the response channel
		Method:       in.GetMethod(),       // needed in unmarshalResponse to look up the response type in the proto registry
		// Payload is left empty; SendMessage will marshal resp into the payload when sending the message
		// Status is left empty; it can be set by messageWithError if needed
	}
	return &Message{
		Msg:     resp,
		Message: msgBuilder.Build(),
	}
}

// AsProto returns the message's already-decoded proto message as type T.
// If the message is nil, or the underlying message cannot be asserted to T,
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

// messageWithError ensures a response message exists and sets the error status.
// If out is nil, a new response message is created based on the in request message;
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

// unmarshalRequest unmarshals the request proto message from the message.
// It uses the method name in the message to look up the Input type from the proto registry.
//
// This function should only be used by internal channel operations.
func unmarshalRequest(in *stream.Message) (proto.Message, error) {
	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(in.GetMethod()))
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s", in.GetMethod())
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get the request message type (Input type)
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Input().FullName())
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s", methodDesc.Input().FullName())
	}
	req := msgType.New().Interface()

	// unmarshal message from the Message.Payload field
	payload := in.GetPayload()
	if len(payload) > 0 {
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, fmt.Errorf("gorums: could not unmarshal request: %w", err)
		}
	}
	return req, nil
}

// unmarshalResponse unmarshals the response proto message from the message.
// It uses the method name in the message to look up the Output type from the proto registry.
//
// This function should only be used by internal channel operations.
func unmarshalResponse(out *stream.Message) (proto.Message, error) {
	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(out.GetMethod()))
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s", out.GetMethod())
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get the response message type (Output type)
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Output().FullName())
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s", methodDesc.Output().FullName())
	}
	resp := msgType.New().Interface()

	// unmarshal message from the Message.Payload field
	payload := out.GetPayload()
	if len(payload) > 0 {
		if err := proto.Unmarshal(payload, resp); err != nil {
			return nil, fmt.Errorf("gorums: could not unmarshal response: %w", err)
		}
	}
	return resp, nil
}
