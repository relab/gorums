package stream

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
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
			got := msg.appendToIncomingContext(context.Background())
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

func TestMessageFromPayloadMatchesBuilder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		ctx         context.Context
		payload     []byte
		wantEntries []string
	}{
		{name: "NilPayload", ctx: context.Background()},
		{name: "EmptyPayload", ctx: context.Background(), payload: []byte{}},
		{name: "PayloadAndMetadata", ctx: metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-id", "42")), payload: []byte("request"), wantEntries: []string{"x-id=42"}},
		{name: "RepeatedMetadataValues", ctx: metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-id", "42", "x-id", "43")), payload: []byte("request"), wantEntries: []string{"x-id=42", "x-id=43"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewMessageFromPayload(tc.ctx, 7, "test.Method", tc.payload)
			want := messageWithBuilder(tc.ctx, tc.payload)
			if !proto.Equal(got, want) {
				t.Errorf("NewMessageFromPayload = %v, want %v", got, want)
			}
			if (got.GetPayload() == nil) != (want.GetPayload() == nil) {
				t.Errorf("payload nil = %v, want %v", got.GetPayload() == nil, want.GetPayload() == nil)
			}
			var entries []string
			for _, entry := range got.GetEntry() {
				entries = append(entries, entry.GetKey()+"="+entry.GetValue())
			}
			if !slices.Equal(entries, tc.wantEntries) {
				t.Errorf("metadata entries = %v, want %v", entries, tc.wantEntries)
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
			md, ok := metadata.FromIncomingContext(tt.msg.appendToIncomingContext(ctx))
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
	if allocs := testing.AllocsPerRun(100, func() { noEntries.appendToIncomingContext(ctx) }); allocs != 0 {
		t.Errorf("AppendToIncomingContext without entries: %v allocs, want 0", allocs)
	}
}
