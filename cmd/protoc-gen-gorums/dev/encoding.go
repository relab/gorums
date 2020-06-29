package dev

import (
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const gorumsContentType = "gorums"

func init() {
	encoding.RegisterCodec(newGorumsCodec())
}

type gorumsMsgType uint8

const (
	gorumsRequest gorumsMsgType = iota + 1
	gorumsResponse
)

type gorumsMessage struct {
	metadata *ordering.Metadata
	message  protoreflect.ProtoMessage
	msgType  gorumsMsgType
}

// newGorumsMessage creates a new gorumsMessage struct for unmarshaling.
// msgType specifies the type of message that should be unmarshaled.
func newGorumsMessage(msgType gorumsMsgType) *gorumsMessage {
	return &gorumsMessage{metadata: &ordering.Metadata{}, msgType: msgType}
}

type gorumsCodec struct {
	marshaler   proto.MarshalOptions
	unmarshaler proto.UnmarshalOptions
}

func newGorumsCodec() *gorumsCodec {
	return &gorumsCodec{
		marshaler:   proto.MarshalOptions{AllowPartial: true},
		unmarshaler: proto.UnmarshalOptions{AllowPartial: true},
	}
}

func (c gorumsCodec) Name() string {
	return gorumsContentType
}

func (c gorumsCodec) String() string {
	return gorumsContentType
}

func (c gorumsCodec) Marshal(m interface{}) (b []byte, err error) {
	switch msg := m.(type) {
	case *gorumsMessage:
		return c.gorumsMarshal(msg)
	case protoreflect.ProtoMessage:
		return c.marshaler.Marshal(msg)
	default:
		return nil, fmt.Errorf("gorumsCodec: don't know how to marshal message of type '%T'", m)
	}
}

// gorumsMarshal marshals a metadata and a data message into a single byte slice.
func (c gorumsCodec) gorumsMarshal(msg *gorumsMessage) (b []byte, err error) {
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

func (c gorumsCodec) Unmarshal(b []byte, m interface{}) (err error) {
	switch msg := m.(type) {
	case *gorumsMessage:
		return c.gorumsUnmarshal(b, msg)
	case protoreflect.ProtoMessage:
		return c.unmarshaler.Unmarshal(b, msg)
	default:
		return fmt.Errorf("gorumsCodec: don't know how to unmarshal message of type '%T'", m)
	}
}

// gorumsUnmarshal unmarshals a metadata and a data message from a byte slice.
func (c gorumsCodec) gorumsUnmarshal(b []byte, msg *gorumsMessage) (err error) {
	mdBuf, mdLen := protowire.ConsumeBytes(b)
	err = c.unmarshaler.Unmarshal(mdBuf, msg.metadata)
	if err != nil {
		return err
	}
	info := orderingMethods[msg.metadata.MethodID]
	switch msg.msgType {
	case gorumsRequest:
		msg.message = info.requestType.New().Interface()
	case gorumsResponse:
		msg.message = info.responseType.New().Interface()
	default:
		return fmt.Errorf("gorumsCodec: Unknown message type.")
	}
	msgBuf, _ := protowire.ConsumeBytes(b[mdLen:])
	err = c.unmarshaler.Unmarshal(msgBuf, msg.message)
	if err != nil {
		return err
	}
	return nil
}
