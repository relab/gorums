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
		t.Fatal(err)
	}
}

func TestUnmarshalGorumsMessage(t *testing.T) {
	buf, err := codec.Marshal(testMsg)
	if err != nil {
		t.Fatal(err)
	}

	msg := &gorumsMessage{metadata: &ordering.Metadata{}, message: &Request{}}
	err = codec.Unmarshal(buf, msg)
	if err != nil {
		t.Fatal(err)
	}

	if msg.metadata.MessageID != 1 || msg.metadata.MethodID != 2 || msg.message.(*Request).Value != "foo bar" {
		t.Fatal("Failed to unmarshal message correctly.")
	}
}

func TestMarshalUnsupportedType(t *testing.T) {
	now := time.Now()
	_, err := codec.Marshal(now)
	if err == nil {
		t.Fatal("Expected error from gorumsEncoder since marshalling unsupported type")
	}
}

// Test that marshaling normal Protobuf message types works
func TestMarshalAndUnmarshalProtobuf(t *testing.T) {
	buf, err := codec.Marshal(testMsg.message)
	if err != nil {
		t.Fatal(err)
	}

	msg := &Request{}
	err = codec.Unmarshal(buf, msg)
	if err != nil {
		t.Fatal(err)
	}

	if msg.Value != testMsg.message.(*Request).Value {
		t.Fatal("Failed to unmarshal message correctly.")
	}
}
