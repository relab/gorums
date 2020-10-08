package gorums

import (
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const gorumsContentType = "gorums"

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

type GorumsCodec struct {
	marshaler   proto.MarshalOptions
	unmarshaler proto.UnmarshalOptions
	methods     map[int32]MethodInfo
}

func NewGorumsCodec(methods map[int32]MethodInfo) *GorumsCodec {
	return &GorumsCodec{
		marshaler:   proto.MarshalOptions{AllowPartial: true},
		unmarshaler: proto.UnmarshalOptions{AllowPartial: true},
		methods:     methods,
	}
}

func (c GorumsCodec) Name() string {
	return gorumsContentType
}

func (c GorumsCodec) String() string {
	return gorumsContentType
}

func (c GorumsCodec) Marshal(m interface{}) (b []byte, err error) {
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
func (c GorumsCodec) gorumsMarshal(msg *Message) (b []byte, err error) {
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

func (c GorumsCodec) Unmarshal(b []byte, m interface{}) (err error) {
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
func (c GorumsCodec) gorumsUnmarshal(b []byte, msg *Message) (err error) {
	mdBuf, mdLen := protowire.ConsumeBytes(b)
	err = c.unmarshaler.Unmarshal(mdBuf, msg.Metadata)
	if err != nil {
		return err
	}
	info, ok := c.methods[msg.Metadata.MethodID]
	if !ok {
		return fmt.Errorf("gorumsCodec: Unknown MethodID")
	}
	switch msg.msgType {
	case gorumsRequestType:
		msg.Message = info.RequestType.New().Interface()
	case gorumsResponseType:
		msg.Message = info.ResponseType.New().Interface()
	default:
		return fmt.Errorf("gorumsCodec: Unknown message type")
	}
	msgBuf, _ := protowire.ConsumeBytes(b[mdLen:])
	err = c.unmarshaler.Unmarshal(msgBuf, msg.Message)
	if err != nil {
		return err
	}
	return nil
}
