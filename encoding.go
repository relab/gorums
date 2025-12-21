package gorums

import (
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func init() {
	encoding.RegisterCodec(NewCodec())
}

// ContentSubtype is the subtype used by gorums when sending messages via gRPC.
const ContentSubtype = "gorums"

type gorumsMsgType uint8

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

// newMessage creates a new Message struct for unmarshaling.
// msgType specifies the message type to be unmarshaled.
func newMessage(msgType gorumsMsgType) *Message {
	return &Message{metadata: &ordering.Metadata{}, msgType: msgType}
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

// Codec is the gRPC codec used by gorums.
type Codec struct {
	marshaler   proto.MarshalOptions
	unmarshaler proto.UnmarshalOptions
}

// NewCodec returns a new Codec.
func NewCodec() *Codec {
	return &Codec{
		marshaler:   proto.MarshalOptions{AllowPartial: true},
		unmarshaler: proto.UnmarshalOptions{AllowPartial: true},
	}
}

// Name returns the name of the Codec.
func (Codec) Name() string {
	return ContentSubtype
}

func (Codec) String() string {
	return ContentSubtype
}

// Marshal marshals the message m into a byte slice.
func (c Codec) Marshal(m any) (b []byte, err error) {
	switch msg := m.(type) {
	case *Message:
		return c.gorumsMarshal(msg)
	case proto.Message:
		return c.marshaler.Marshal(msg)
	default:
		return nil, fmt.Errorf("gorums: cannot marshal message of type '%T'", m)
	}
}

// gorumsMarshal marshals a metadata and a data message into a single byte slice.
func (c Codec) gorumsMarshal(msg *Message) (b []byte, err error) {
	mdSize := c.marshaler.Size(msg.metadata)
	b = protowire.AppendVarint(b, uint64(mdSize))
	b, err = c.marshaler.MarshalAppend(b, msg.metadata)
	if err != nil {
		return nil, err
	}

	msgSize := c.marshaler.Size(msg.message)
	b = protowire.AppendVarint(b, uint64(msgSize))
	b, err = c.marshaler.MarshalAppend(b, msg.message)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Unmarshal unmarshals a byte slice into m.
func (c Codec) Unmarshal(b []byte, m any) (err error) {
	switch msg := m.(type) {
	case *Message:
		return c.gorumsUnmarshal(b, msg)
	case proto.Message:
		return c.unmarshaler.Unmarshal(b, msg)
	default:
		return fmt.Errorf("gorums: cannot unmarshal message of type '%T'", m)
	}
}

// gorumsUnmarshal extracts metadata and message data from b and places the result in msg.
func (c Codec) gorumsUnmarshal(b []byte, msg *Message) (err error) {
	// unmarshal metadata
	mdBuf, mdLen := protowire.ConsumeBytes(b)
	err = c.unmarshaler.Unmarshal(mdBuf, msg.metadata)
	if err != nil {
		return fmt.Errorf("gorums: could not unmarshal metadata: %w", err)
	}

	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(msg.GetMethod()))
	if err != nil {
		// err is a NotFound error with no method name information; return a more informative error
		return fmt.Errorf("gorums: could not find method descriptor for %s", msg.GetMethod())
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
		return fmt.Errorf("gorums: unknown message type %d", msg.msgType)
	}

	// now get the message type from the types registry
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(messageName)
	if err != nil {
		// err is a NotFound error with no message name information; return a more informative error
		return fmt.Errorf("gorums: could not find message type %s", messageName)
	}
	msg.message = msgType.New().Interface()

	// unmarshal message
	msgBuf, _ := protowire.ConsumeBytes(b[mdLen:])
	return c.unmarshaler.Unmarshal(msgBuf, msg.message)
}
