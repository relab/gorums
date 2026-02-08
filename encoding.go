package gorums

import (
	"context"
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Message encapsulates a protobuf message and metadata.
//
// This struct should be used by generated code only.
type Message struct {
	metadata *ordering.Metadata
	message  proto.Message
}

// NewResponseMessage creates a new Gorums Message for the given metadata and response message.
//
// This function should be used by generated code only.
func NewResponseMessage(md *ordering.Metadata, resp proto.Message) *Message {
	return &Message{metadata: md, message: resp}
}

// AsProto returns msg's underlying protobuf message of the specified type T.
// If msg is nil or the contained message is not of type T, the zero value of T is returned.
func AsProto[T proto.Message](msg *Message) T {
	var zero T
	if msg == nil || msg.message == nil {
		return zero
	}
	if req, ok := msg.message.(T); ok {
		return req
	}
	return zero
}

// GetProtoMessage returns the protobuf message contained in the Message.
func (m *Message) GetProtoMessage() proto.Message {
	if m == nil {
		return nil
	}
	return m.message
}

// GetMetadata returns the metadata of the message.
func (m *Message) GetMetadata() *ordering.Metadata {
	if m == nil {
		return nil
	}
	return m.metadata
}

// GetMethod returns the method name from the message metadata.
func (m *Message) GetMethod() string {
	if m == nil {
		return "nil"
	}
	return m.metadata.GetMethod()
}

// GetMessageID returns the message ID from the message metadata.
func (m *Message) GetMessageID() uint64 {
	if m == nil {
		return 0
	}
	return m.metadata.GetMessageSeqNo()
}

func (m *Message) GetStatus() *status.Status {
	if m == nil {
		return status.New(codes.Unknown, "nil message")
	}
	return status.FromProto(m.metadata.GetStatus())
}

// setError sets the error status in the message metadata in preparation for sending
// the response to the client. The provided error may include several wrapped errors.
// If err is nil, the status is set to OK.
// This method should be called just prior to sending the response to the client.
func (m *Message) setError(err error) {
	errStatus, ok := status.FromError(err)
	if !ok {
		errStatus = status.New(codes.Unknown, err.Error())
	}
	m.metadata.SetStatus(errStatus.Proto())
}

// responseWithError ensures a response message exists and sets the error status.
// If msg is nil, a new response message is created using the provided metadata.
// This is used by the server to send error responses back to the client.
func responseWithError(msg *Message, md *ordering.Metadata, err error) *Message {
	if msg == nil {
		msg = NewResponseMessage(md, nil)
	}
	msg.setError(err)
	return msg
}

// marshalRequest marshals the request proto message into metadata for type-safe Send.
// It marshals the proto message into the metadata's message_data field.
//
// This function should be used by client-side operations only.
func marshalRequest(ctx context.Context, msgID uint64, method string, req proto.Message) (*ordering.Metadata, error) {
	return ordering.NewMetadata(ctx, msgID, method, req)
}

// marshalResponse marshals the response message into metadata for type-safe Send.
// It clones the metadata to avoid race conditions with concurrent send operations.
//
// This function should be used by server-side operations only.
func marshalResponse(msg *Message) (*ordering.Metadata, error) {
	if msg == nil {
		return nil, nil
	}
	// Clone metadata to avoid race with concurrent send operations
	md := proto.CloneOf(msg.metadata)
	if msg.message != nil {
		msgData, err := proto.Marshal(msg.message)
		if err != nil {
			return nil, err
		}
		md.SetMessageData(msgData)
	}
	return md, nil
}

// unmarshalRequest unmarshals the request proto message from metadata.
// It uses the method name in metadata to look up the Input type from the proto registry.
// Returns a *Message suitable for passing to handlers.
//
// This function should be used by server-side operations only.
func unmarshalRequest(md *ordering.Metadata) (*Message, error) {
	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(md.GetMethod()))
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s", md.GetMethod())
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get the request message type (Input type)
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Input().FullName())
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s", methodDesc.Input().FullName())
	}
	req := msgType.New().Interface()

	// unmarshal message from metadata.message_data
	msgData := md.GetMessageData()
	if len(msgData) > 0 {
		if err := proto.Unmarshal(msgData, req); err != nil {
			return nil, fmt.Errorf("gorums: could not unmarshal request: %w", err)
		}
	}
	return &Message{metadata: md, message: req}, nil
}

// unmarshalResponse unmarshals the response proto message from metadata.
// It uses the method name in metadata to look up the Output type from the proto registry.
//
// This function should be used by internal channel operations only.
func unmarshalResponse(md *ordering.Metadata) (proto.Message, error) {
	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(md.GetMethod()))
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s", md.GetMethod())
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get the response message type (Output type)
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Output().FullName())
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s", methodDesc.Output().FullName())
	}
	resp := msgType.New().Interface()

	// unmarshal message from metadata.message_data
	msgData := md.GetMessageData()
	if len(msgData) > 0 {
		if err := proto.Unmarshal(msgData, resp); err != nil {
			return nil, fmt.Errorf("gorums: could not unmarshal response: %w", err)
		}
	}
	return resp, nil
}
