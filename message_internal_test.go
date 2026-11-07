package gorums

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMessageNewResponse(t *testing.T) {
	in := stream.Message_builder{
		MessageSeqNo: 100,
		Method:       mock.GetValueMethod,
		Payload:      []byte("request payload"),
		Entry:        []*stream.MetadataEntry{stream.MetadataEntry_builder{Key: "key1", Value: "val1"}.Build()},
	}.Build()
	payload, err := proto.Marshal(pb.String("response"))
	if err != nil {
		t.Fatal(err)
	}
	// response returns the expected envelope; it carries only the request's
	// message ID and method, never its payload or metadata.
	response := func(payload []byte, code codes.Code, msg string) *stream.Message {
		out := stream.Message_builder{MessageSeqNo: 100, Method: mock.GetValueMethod, Payload: payload}.Build()
		if code != codes.OK {
			out.SetStatus(status.New(code, msg).Proto())
		}
		return out
	}

	tests := []struct {
		name string
		resp proto.Message
		err  error
		want *stream.Message
	}{
		{name: "NilResp", resp: nil, err: nil, want: response(nil, codes.OK, "")},
		{name: "EmptyResp", resp: &emptypb.Empty{}, err: nil, want: response([]byte{}, codes.OK, "")},
		{name: "Resp", resp: pb.String("response"), err: nil, want: response(payload, codes.OK, "")},
		{name: "StatusErr", resp: nil, err: status.Error(codes.NotFound, "missing"), want: response(nil, codes.NotFound, "missing")},
		{name: "PlainErr", resp: nil, err: errors.New("failed"), want: response(nil, codes.Unknown, "failed")},
		{name: "RespAndErr", resp: pb.String("response"), err: errors.New("failed"), want: response(nil, codes.Unknown, "failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newResponse(in, tt.resp, tt.err)
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("newResponse mismatch (-want, +got):\n%s", diff)
			}
			if (got.GetPayload() == nil) != (tt.want.GetPayload() == nil) {
				t.Errorf("payload nil = %v, want %v", got.GetPayload() == nil, tt.want.GetPayload() == nil)
			}
		})
	}

	t.Run("MarshalErr", func(t *testing.T) {
		got := newResponse(in, pb.String("\xff"), nil) // invalid UTF-8 fails to marshal
		if len(got.GetPayload()) != 0 {
			t.Errorf("payload = %q, want empty", got.GetPayload())
		}
		if code := status.FromProto(got.GetStatus()).Code(); code != codes.Unknown {
			t.Errorf("status code = %v, want %v", code, codes.Unknown)
		}
	})
}
