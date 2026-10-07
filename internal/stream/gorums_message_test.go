package stream

import (
	"context"
	"reflect"
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

// TestMessageAppendToIncomingContext verifies that AppendToIncomingContext
// adds a message's entries to a copy of the incoming metadata and leaves the
// original metadata unchanged, and that a message without entries returns ctx
// as it is, without allocating.
func TestMessageAppendToIncomingContext(t *testing.T) {
	withEntries := NewMessageFromPayload(metadata.NewOutgoingContext(t.Context(), metadata.Pairs("x-role", "replica")), 1, "test.Method", nil)
	noEntries := NewMessageFromPayload(t.Context(), 2, "test.Method", nil)
	tests := []struct {
		name     string
		incoming metadata.MD // nil means no incoming metadata
		msg      *Message
		want     metadata.MD // nil means no incoming metadata
	}{
		{"NoEntriesKeepsIncoming", metadata.Pairs("authority", "peer"), noEntries, metadata.Pairs("authority", "peer")},
		{"NoEntriesNoIncoming", nil, noEntries, nil},
		{"EntriesAppendToIncoming", metadata.Pairs("authority", "peer"), withEntries, metadata.Pairs("authority", "peer", "x-role", "replica")},
		{"EntriesNoIncoming", nil, withEntries, metadata.Pairs("x-role", "replica")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			var original metadata.MD
			if tt.incoming != nil {
				original = tt.incoming.Copy()
				ctx = metadata.NewIncomingContext(ctx, tt.incoming)
			}
			md, ok := metadata.FromIncomingContext(tt.msg.AppendToIncomingContext(ctx))
			if ok != (tt.want != nil) {
				t.Fatalf("incoming metadata present = %v, want %v", ok, tt.want != nil)
			}
			if ok && !reflect.DeepEqual(md, tt.want) {
				t.Errorf("incoming metadata = %v, want %v", md, tt.want)
			}
			if tt.incoming != nil && !reflect.DeepEqual(tt.incoming, original) {
				t.Errorf("original metadata changed to %v, want %v", tt.incoming, original)
			}
		})
	}

	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authority", "peer", "content-type", "application/grpc"))
	if allocs := testing.AllocsPerRun(100, func() { noEntries.AppendToIncomingContext(ctx) }); allocs != 0 {
		t.Errorf("AppendToIncomingContext without entries: %v allocs, want 0", allocs)
	}
}
