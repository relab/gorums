package gorums

import (
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

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
	Metadata *ordering.Metadata
	Message  proto.Message
	msgType  gorumsMsgType
}

// newMessage creates a new Message struct for unmarshaling.
// msgType specifies the message type to be unmarshaled.
func newMessage(msgType gorumsMsgType) *Message {
	return &Message{Metadata: &ordering.Metadata{}, msgType: msgType}
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
func (c Codec) Name() string {
	return ContentSubtype
}

func (c Codec) String() string {
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
	mdSize := c.marshaler.Size(msg.Metadata)
	b = protowire.AppendVarint(b, uint64(mdSize))
	b, err = c.marshaler.MarshalAppend(b, msg.Metadata)
	if err != nil {
		return nil, err
	}

	msgSize := c.marshaler.Size(msg.Message)
	b = protowire.AppendVarint(b, uint64(msgSize))
	b, err = c.marshaler.MarshalAppend(b, msg.Message)
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
	err = c.unmarshaler.Unmarshal(mdBuf, msg.Metadata)
	if err != nil {
		return fmt.Errorf("gorums: could not unmarshal metadata: %w", err)
	}

	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(msg.Metadata.GetMethod()))
	if err != nil {
		// err is a NotFound error with no method name information; return a more informative error
		return fmt.Errorf("gorums: could not find method descriptor for %s", msg.Metadata.GetMethod())
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
	msg.Message = msgType.New().Interface()

	// unmarshal message
	msgBuf, _ := protowire.ConsumeBytes(b[mdLen:])
	return c.unmarshaler.Unmarshal(msgBuf, msg.Message)
}
