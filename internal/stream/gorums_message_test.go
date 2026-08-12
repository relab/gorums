package stream

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestMessageConstructorsPreservePayloadAndMetadata(t *testing.T) {
	ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("x-request-id", "42", "x-role", "replica"))
	payload := []byte("payload")

	fromProto, err := NewMessage(ctx, 7, "test.Method", nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	fromPayload := NewMessageFromPayload(ctx, 8, "test.Method", payload)

	if got := fromProto.GetPayload(); len(got) != 0 {
		t.Fatalf("NewMessage payload = %q, want empty payload", got)
	}
	if got := string(fromPayload.GetPayload()); got != string(payload) {
		t.Fatalf("NewMessageFromPayload payload = %q, want %q", got, payload)
	}
	for name, msg := range map[string]*Message{"proto": fromProto, "payload": fromPayload} {
		t.Run(name, func(t *testing.T) {
			got := msg.AppendToIncomingContext(context.Background())
			md, ok := metadata.FromIncomingContext(got)
			if !ok {
				t.Fatal("missing incoming metadata")
			}
			if values := md.Get("x-request-id"); len(values) != 1 || values[0] != "42" {
				t.Fatalf("x-request-id = %v, want [42]", values)
			}
			if values := md.Get("x-role"); len(values) != 1 || values[0] != "replica" {
				t.Fatalf("x-role = %v, want [replica]", values)
			}
		})
	}
}
