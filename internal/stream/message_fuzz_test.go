package stream

import (
	"bytes"
	"reflect"
	"testing"
	"unicode/utf8"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func FuzzMessageMetadata(f *testing.F) {
	f.Add("x-key", "first", "second", []byte{1, 2, 3}, uint64(7))
	f.Add("X-Key", "", "\x00", []byte{}, ^uint64(0))
	f.Fuzz(func(t *testing.T, key, first, second string, payload []byte, id uint64) {
		if len(key)+len(first)+len(second)+len(payload) > 64<<10 {
			t.Skip()
		}
		// The envelope currently requires UTF-8 in metadata. Binary metadata
		// will exercise the wire path when the envelope supports it.
		if !utf8.ValidString(key) || !utf8.ValidString(first) || !utf8.ValidString(second) {
			t.Skip()
		}
		outgoing := metadata.Pairs(key, first, key, second)
		ctx := metadata.NewOutgoingContext(t.Context(), outgoing)
		msg := NewRequestFromPayload(ctx, id, "test.Method", payload)
		wire, err := proto.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		decoded := new(Message)
		if err := proto.Unmarshal(wire, decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.GetMessageSeqNo() != id || decoded.GetMethod() != "test.Method" || !bytes.Equal(decoded.GetPayload(), payload) {
			t.Fatal("envelope changed payload or routing")
		}
		incoming := metadata.Pairs(key, "existing", "unrelated", "keep")
		parent := metadata.NewIncomingContext(t.Context(), incoming)
		snapshot := incoming.Copy()
		want := metadata.Join(snapshot, outgoing)
		got, _ := metadata.FromIncomingContext(decoded.appendToIncomingContext(parent))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("metadata = %#v, want %#v", got, want)
		}
		original, _ := metadata.FromIncomingContext(parent)
		if !reflect.DeepEqual(original, snapshot) {
			t.Fatalf("parent metadata = %#v, want %#v", original, snapshot)
		}
	})
}
