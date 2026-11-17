package impl

import (
	"testing"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func FuzzMarshaling(f *testing.F) {
	for _, method := range []string{mock.TestMethod, "stream.Gorums.NodeStream", "stream.MetadataEntry", "stream.Gorums", "stream.Message.entry", "missing.Method", ""} {
		for _, payload := range [][]byte{nil, {8, 7}, {10, 1, 'a'}, {255}} {
			f.Add(method, payload)
		}
	}
	f.Fuzz(func(t *testing.T, method string, payload []byte) {
		if len(method) > 1024 || len(payload) > 64<<10 {
			t.Skip()
		}
		in := stream.NewRequestFromPayload(t.Context(), 1, method, payload)
		for _, decode := range []func(*stream.Message) (proto.Message, error){UnmarshalRequest, unmarshalResponse} {
			got, err := decode(in)
			var want proto.Message
			switch method {
			case mock.TestMethod:
				want = new(pb.StringValue)
			case "stream.Gorums.NodeStream":
				want = new(stream.Message)
			case "stream.MetadataEntry", "stream.Gorums", "stream.Message.entry", "missing.Method", "":
				if err == nil {
					t.Fatalf("accepted non-method %q", method)
				}
				continue
			default:
				// Arbitrary input may also name another registered method. If decoding
				// succeeds, re-encoding it must yield an equivalent decoded message.
				if err == nil {
					encoded, e := proto.Marshal(got)
					if e != nil {
						t.Fatal(e)
					}
					roundTrip, e := decode(stream.NewRequestFromPayload(t.Context(), 1, method, encoded))
					if e != nil || !proto.Equal(got, roundTrip) {
						t.Fatalf("round trip = %v, %v; original %v", roundTrip, e, got)
					}
				}
				continue
			}
			wantErr := proto.Unmarshal(payload, want)
			if (err != nil) != (wantErr != nil) {
				t.Fatalf("decode(%q): %v, direct decode: %v", method, err, wantErr)
			}
			if err == nil && !proto.Equal(got, want) {
				t.Fatalf("decode(%q) = %v (%T), want %v (%T)", method, got, got, want, want)
			}
		}
	})
}
