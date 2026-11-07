package stream

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

var benchmarkMessage *Message

// BenchmarkMessageConstruction compares equivalent request envelopes with and
// without outgoing metadata. The payload is prepared outside the timed loop.
func BenchmarkMessageConstruction(b *testing.B) {
	payload := []byte("request payload")
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "NoMetadata", ctx: context.Background()},
		{name: "OneEntry", ctx: metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-id", "42"))},
		{name: "ThreeEntries", ctx: metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-id", "42", "x-role", "replica", "x-zone", "one"))},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.Run("Production", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					benchmarkMessage = NewMessageFromPayload(tc.ctx, 7, "test.Method", payload)
				}
			})
			b.Run("Builder", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					benchmarkMessage = messageWithBuilder(tc.ctx, payload)
				}
			})
			b.Run("Setters", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					benchmarkMessage = messageWithSetters(tc.ctx, payload)
				}
			})
		})
	}
}

func messageWithBuilder(ctx context.Context, payload []byte) *Message {
	builder := Message_builder{MessageSeqNo: 7, Method: "test.Method", Payload: payload}
	md, _ := metadata.FromOutgoingContext(ctx)
	for k, vv := range md {
		for _, v := range vv {
			builder.Entry = append(builder.Entry, MetadataEntry_builder{Key: k, Value: v}.Build())
		}
	}
	return builder.Build()
}

func messageWithSetters(ctx context.Context, payload []byte) *Message {
	msg := &Message{}
	msg.SetMessageSeqNo(7)
	msg.SetMethod("test.Method")
	msg.SetPayload(payload)
	md, _ := metadata.FromOutgoingContext(ctx)
	var entries []*MetadataEntry
	for k, vv := range md {
		for _, v := range vv {
			entry := &MetadataEntry{}
			entry.SetKey(k)
			entry.SetValue(v)
			entries = append(entries, entry)
		}
	}
	if len(entries) != 0 {
		msg.SetEntry(entries)
	}
	return msg
}
