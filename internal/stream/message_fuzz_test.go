package stream

import (
	"bytes"
	"reflect"
	"testing"

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
		outgoing := metadata.Pairs(key, first, key, second)
		ctx := metadata.NewOutgoingContext(t.Context(), outgoing)
		msg := NewRequestFromPayload(ctx, id, "test.Method", payload)
		if msg.GetMessageSeqNo() != id || msg.GetMethod() != "test.Method" || !bytes.Equal(msg.GetPayload(), payload) {
			t.Fatal("envelope changed payload or routing")
		}
		incoming := metadata.Pairs(key, "existing", "unrelated", "keep")
		parent := metadata.NewIncomingContext(t.Context(), incoming)
		want := metadata.Join(incoming, outgoing)
		got, _ := metadata.FromIncomingContext(msg.appendToIncomingContext(parent))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("metadata = %#v, want %#v", got, want)
		}
		original, _ := metadata.FromIncomingContext(parent)
		if !reflect.DeepEqual(original, incoming) {
			t.Fatal("changed parent metadata")
		}
		// Mutating the returned context must also leave its parent unchanged.
		for k := range got {
			got[k][0] = "changed"
		}
		original, _ = metadata.FromIncomingContext(parent)
		if !reflect.DeepEqual(original, incoming) {
			t.Fatal("metadata aliases parent")
		}
		copy, err := NewRequest(ctx, id, "test.Method", new(Message))
		if err != nil {
			t.Fatal(err)
		}
		decoded := new(Message)
		if err := proto.Unmarshal(copy.GetPayload(), decoded); err != nil {
			t.Fatal(err)
		}
	})
}
