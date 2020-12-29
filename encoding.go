package gorums

import (
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const ContentSubtype = "gorums"

type gorumsMsgType uint8

const (
	gorumsRequestType gorumsMsgType = iota + 1
	gorumsResponseType
)

type Message struct {
	Metadata *ordering.Metadata
	Message  protoreflect.ProtoMessage
	msgType  gorumsMsgType
}

// newGorumsMessage creates a new gorumsMessage struct for unmarshaling.
// msgType specifies the type of message that should be unmarshaled.
func newGorumsMessage(msgType gorumsMsgType) *Message {
	return &Message{Metadata: &ordering.Metadata{}, msgType: msgType}
}

type Codec struct {
	marshaler   proto.MarshalOptions
	unmarshaler proto.UnmarshalOptions
}

func NewCodec() *Codec {
	return &Codec{
		marshaler:   proto.MarshalOptions{AllowPartial: true},
		unmarshaler: proto.UnmarshalOptions{AllowPartial: true},
	}
}

func (c Codec) Name() string {
	return ContentSubtype
}

func (c Codec) String() string {
	return ContentSubtype
}

func (c Codec) Marshal(m interface{}) (b []byte, err error) {
	switch msg := m.(type) {
	case *Message:
		return c.gorumsMarshal(msg)
	case protoreflect.ProtoMessage:
		return c.marshaler.Marshal(msg)
	default:
		return nil, fmt.Errorf("gorumsCodec: don't know how to marshal message of type '%T'", m)
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

func (c Codec) Unmarshal(b []byte, m interface{}) (err error) {
	switch msg := m.(type) {
	case *Message:
		return c.gorumsUnmarshal(b, msg)
	case protoreflect.ProtoMessage:
		return c.unmarshaler.Unmarshal(b, msg)
	default:
		return fmt.Errorf("gorumsCodec: don't know how to unmarshal message of type '%T'", m)
	}
}

// gorumsUnmarshal unmarshals a metadata and a data message from a byte slice.
func (c Codec) gorumsUnmarshal(b []byte, msg *Message) (err error) {
	// unmarshal metadata
	mdBuf, mdLen := protowire.ConsumeBytes(b)
	err = c.unmarshaler.Unmarshal(mdBuf, msg.Metadata)
	if err != nil {
		return err
	}

	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(msg.Metadata.Method))
	if err != nil {
		return err
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get message name depending on whether we are creating a request or response message
	var messageName protoreflect.FullName
	switch msg.msgType {
	case gorumsRequestType:
		messageName = methodDesc.Input().FullName()
	case gorumsResponseType:
		messageName = methodDesc.Output().FullName()
	default:
		return fmt.Errorf("gorumsCodec: Unknown message type")
	}

	// now get the message type from the types registry
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(messageName)
	if err != nil {
		return err
	}
	msg.Message = msgType.New().Interface()

	// unmarshal message
	msgBuf, _ := protowire.ConsumeBytes(b[mdLen:])
	err = c.unmarshaler.Unmarshal(msgBuf, msg.Message)

	return err
}
