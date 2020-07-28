package gorums

import (
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
)

var methods = map[int32]MethodInfo{
	2: {
		requestType:  (&ordering.Metadata{}).ProtoReflect(),
		responseType: (&ordering.Metadata{}).ProtoReflect(),
	},
}

var (
	testMsg = &gorumsMessage{
		metadata: &ordering.Metadata{MessageID: 1, MethodID: 2},
		message:  &ordering.Metadata{MessageID: 42},
	}
	codec = NewGorumsCodec(methods)
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

	msg := newGorumsMessage(gorumsRequest)
	err = codec.Unmarshal(buf, msg)
	if err != nil {
		t.Fatal(err)
	}

	if msg.metadata.MessageID != 1 || msg.metadata.MethodID != 2 || msg.message.(*ordering.Metadata).MessageID != 42 {
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

	msg := &ordering.Metadata{}
	err = codec.Unmarshal(buf, msg)
	if err != nil {
		t.Fatal(err)
	}

	if msg.MethodID != testMsg.message.(*ordering.Metadata).MethodID {
		t.Fatal("Failed to unmarshal message correctly.")
	}
}
