package dev

import (
	"testing"

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
	_, err := codec.gorumsMarshal(testMsg)
	if err != nil {
		t.Error(err)
	}
}

func TestUnmarshalGorumsMessage(t *testing.T) {
	buf, err := codec.gorumsMarshal(testMsg)
	if err != nil {
		t.Error(err)
	}

	msg := &gorumsMessage{metadata: &ordering.Metadata{}, message: &Request{}}
	err = codec.gorumsUnmarshal(buf, msg)
	if err != nil {
		t.Error(err)
	}

	if msg.metadata.MessageID != 1 || msg.metadata.MethodID != 2 || msg.message.(*Request).Value != "foo bar" {
		t.Errorf("Failed to unmarshal message correctly.")
	}
}
