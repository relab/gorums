package impl

import (
	"errors"
	"testing"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// TestMarshalingUnmarshalMethodName verifies that request and response
// unmarshaling resolve the message type from the method name, and that a
// method name that is unknown or names a registered descriptor other than a
// method fails with an error instead of a panic.
func TestMarshalingUnmarshalMethodName(t *testing.T) {
	payload, err := proto.Marshal(stream.Message_builder{MessageSeqNo: 7}.Build())
	if err != nil {
		t.Fatal(err)
	}
	unmarshalers := []struct {
		name      string
		unmarshal func(*stream.Message) (proto.Message, error)
	}{
		{name: "Request", unmarshal: UnmarshalRequest},
		{name: "Response", unmarshal: unmarshalResponse},
	}
	tests := []struct {
		name         string
		method       string
		wantNotFound bool
		wantErr      bool
	}{
		{name: "Method", method: "stream.Gorums.NodeStream"},
		{name: "UnknownMethod", method: "stream.Gorums.Missing", wantNotFound: true, wantErr: true},
		{name: "MessageName", method: "stream.MetadataEntry", wantErr: true},
		{name: "ServiceName", method: "stream.Gorums", wantErr: true},
		{name: "FieldName", method: "stream.Message.entry", wantErr: true},
	}
	for _, u := range unmarshalers {
		for _, tt := range tests {
			t.Run(u.name+"/"+tt.name, func(t *testing.T) {
				in := stream.Message_builder{Method: tt.method, Payload: payload}.Build()
				msg, err := u.unmarshal(in)
				if (err != nil) != tt.wantErr {
					t.Fatalf("unmarshal(%q) error = %v, want error %t", tt.method, err, tt.wantErr)
				}
				if got := errors.Is(err, protoregistry.NotFound); got != tt.wantNotFound {
					t.Errorf("errors.Is(%v, protoregistry.NotFound) = %t, want %t", err, got, tt.wantNotFound)
				}
				if tt.wantErr {
					return
				}
				got, ok := msg.(*stream.Message)
				if !ok || got.GetMessageSeqNo() != 7 {
					t.Errorf("unmarshal(%q) = %v, want a stream.Message with MessageSeqNo 7", tt.method, msg)
				}
			})
		}
	}
}
