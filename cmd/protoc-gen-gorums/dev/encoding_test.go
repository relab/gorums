package dev

import (
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
)

var (
	testMsg = &gorumsMessage{
		metadata: &ordering.Metadata{MessageID: 1, MethodID: 2},
		message:  &Request{Value: "foo bar"},
	}
	codec = newGorumsCodec()
)

func TestMarshalGorumsMessage(t *testing.T) {
	_, err := codec.Marshal(testMsg)
	if err != nil {
		t.Error(err)
	}
}

func TestUnmarshalGorumsMessage(t *testing.T) {
	buf, err := codec.Marshal(testMsg)
	if err != nil {
		t.Error(err)
	}

	msg := &gorumsMessage{metadata: &ordering.Metadata{}, message: &Request{}}
	err = codec.Unmarshal(buf, msg)
	if err != nil {
		t.Error(err)
	}

	if msg.metadata.MessageID != 1 || msg.metadata.MethodID != 2 || msg.message.(*Request).Value != "foo bar" {
		t.Errorf("Failed to unmarshal message correctly.")
	}
}

func TestMarshalUnsupportedType(t *testing.T) {
	now := time.Now()
	_, err := codec.Marshal(now)
	if err == nil {
		t.Error("Expected error from gorumsEncoder since marshalling unsupported type")
	}
}
