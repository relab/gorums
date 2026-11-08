package stream

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
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

func TestMessageNewResponse(t *testing.T) {
	in := Message_builder{
		MessageSeqNo: 100,
		Method:       "test.Method",
		Payload:      []byte("request payload"),
		Entry:        []*MetadataEntry{MetadataEntry_builder{Key: "key1", Value: "val1"}.Build()},
	}.Build()
	payload, err := proto.Marshal(pb.String("response"))
	if err != nil {
		t.Fatal(err)
	}
	// response returns the expected envelope; it carries only the request's
	// message ID and method, never its payload or metadata.
	response := func(payload []byte, code codes.Code, msg string) *Message {
		out := Message_builder{MessageSeqNo: 100, Method: "test.Method", Payload: payload}.Build()
		if code != codes.OK {
			out.SetStatus(status.New(code, msg).Proto())
		}
		return out
	}

	tests := []struct {
		name string
		resp proto.Message
		err  error
		want *Message
	}{
		{name: "NilResp", resp: nil, err: nil, want: response(nil, codes.OK, "")},
		{name: "Resp", resp: pb.String("response"), err: nil, want: response(payload, codes.OK, "")},
		{name: "StatusErr", resp: nil, err: status.Error(codes.NotFound, "missing"), want: response(nil, codes.NotFound, "missing")},
		{name: "PlainErr", resp: nil, err: errors.New("failed"), want: response(nil, codes.Unknown, "failed")},
		{name: "RespAndErr", resp: pb.String("response"), err: errors.New("failed"), want: response(nil, codes.Unknown, "failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewResponse(in, tt.resp, tt.err)
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("NewResponse mismatch (-want, +got):\n%s", diff)
			}
		})
	}

	t.Run("MarshalErr", func(t *testing.T) {
		got := NewResponse(in, pb.String("\xff"), nil) // invalid UTF-8 fails to marshal
		if len(got.GetPayload()) != 0 {
			t.Errorf("payload = %q, want empty", got.GetPayload())
		}
		if code := status.FromProto(got.GetStatus()).Code(); code != codes.Unknown {
			t.Errorf("status code = %v, want %v", code, codes.Unknown)
		}
	})
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
