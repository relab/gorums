package gorums

import (
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type gorumsMsgType uint32

const (
	requestType gorumsMsgType = iota + 1
	responseType
)

// Message encapsulates a protobuf message and metadata.
//
// This struct should be used by generated code only.
type Message struct {
	metadata *ordering.Metadata
	message  proto.Message
	msgType  gorumsMsgType
}

// NewRequestMessage creates a new Gorums Message for the given metadata and request message.
//
// This function should be used by generated code and tests only.
func NewRequestMessage(md *ordering.Metadata, req proto.Message) *Message {
	return &Message{metadata: md, message: req, msgType: requestType}
}

// NewResponseMessage creates a new Gorums Message for the given metadata and response message.
//
// This function should be used by generated code only.
func NewResponseMessage(md *ordering.Metadata, resp proto.Message) *Message {
	return &Message{metadata: md, message: resp, msgType: responseType}
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

// toMetadata serializes the application message into the metadata's payload
// field and returns the metadata, ready for sending via the type-safe Send method.
func (m *Message) toMetadata() (*ordering.Metadata, error) {
	md := m.metadata
	md.SetMsgType(uint32(m.msgType))
	if m.message != nil {
		b, err := proto.MarshalOptions{AllowPartial: true}.Marshal(m.message)
		if err != nil {
			return nil, fmt.Errorf("gorums: failed to marshal payload: %w", err)
		}
		md.SetPayload(b)
	}
	return md, nil
}

// fromMetadata reconstructs a Message from a received Metadata by deserializing
// the payload bytes into the appropriate protobuf message type, determined by
// the method descriptor and message type (request or response) in the metadata.
func fromMetadata(md *ordering.Metadata) (*Message, error) {
	msg := &Message{
		metadata: md,
		msgType:  gorumsMsgType(md.GetMsgType()),
	}

	method := msg.GetMethod()
	if method == "" || method == "nil" {
		return msg, nil
	}

	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(method))
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s", method)
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get message name depending on whether we are creating a request or response message
	var messageName protoreflect.FullName
	switch msg.msgType {
	case requestType:
		messageName = methodDesc.Input().FullName()
	case responseType:
		messageName = methodDesc.Output().FullName()
	default:
		return nil, fmt.Errorf("gorums: unknown message type %d", msg.msgType)
	}

	// get the message type from the types registry
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(messageName)
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s", messageName)
	}
	msg.message = msgType.New().Interface()

	// unmarshal payload into the message
	payload := md.GetPayload()
	if len(payload) > 0 {
		if err := (proto.UnmarshalOptions{AllowPartial: true}).Unmarshal(payload, msg.message); err != nil {
			return nil, fmt.Errorf("gorums: failed to unmarshal payload: %w", err)
		}
	}
	return msg, nil
}
