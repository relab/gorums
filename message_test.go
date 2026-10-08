package gorums_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestNewResponseMessage(t *testing.T) {
	req := dev.Request_builder{Value: "request"}.Build()
	resp := dev.Response_builder{Result: 42}.Build()

	streamIn := stream.Message_builder{
		MessageSeqNo: 100,
		Method:       mock.GetValueMethod,
		Payload:      []byte("request payload"),
		Entry:        []*stream.MetadataEntry{stream.MetadataEntry_builder{Key: "key1", Value: "val1"}.Build()},
	}.Build()
	streamOut := stream.Message_builder{MessageSeqNo: 100, Method: mock.GetValueMethod}.Build()

	tests := []struct {
		name string
		in   *gorums.Message
		resp *dev.Response
		want *gorums.Message
	}{
		{
			name: "NilIn/NilResp/NilOut",
			in:   nil,
			resp: nil,
			want: nil,
		},
		{
			name: "NilIn/Resp/NilOut",
			in:   nil,
			resp: resp,
			want: nil,
		},
		{
			name: "NilReq/NilResp/StreamIn/StreamOut",
			in:   &gorums.Message{Proto: nil, Message: streamIn},
			resp: nil,
			want: &gorums.Message{Proto: (*dev.Response)(nil), Message: streamOut},
		},
		{
			name: "NilReq/Resp/StreamIn/StreamOut",
			in:   &gorums.Message{Proto: nil, Message: streamIn},
			resp: resp,
			want: &gorums.Message{Proto: resp, Message: streamOut},
		},
		{
			name: "Req/NilResp/StreamIn/StreamOut",
			in:   &gorums.Message{Proto: req, Message: streamIn},
			resp: nil,
			want: &gorums.Message{Proto: (*dev.Response)(nil), Message: streamOut},
		},
		{
			name: "Req/Resp/StreamIn/StreamOut",
			in:   &gorums.Message{Proto: req, Message: streamIn},
			resp: resp,
			want: &gorums.Message{Proto: resp, Message: streamOut},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gorums.NewResponseMessage(tt.in, tt.resp)
			if tt.want == nil {
				if got != nil {
					t.Errorf("NewResponseMessage returned %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("NewResponseMessage returned nil, want non-nil")
			}
			if (tt.want.Proto == nil) != (got.Proto == nil) {
				t.Errorf("Msg field: want nil=%v, got nil=%v", tt.want.Proto == nil, got.Proto == nil)
			} else if tt.want.Proto != nil && got.Proto != nil {
				if diff := cmp.Diff(tt.want.Proto, got.Proto, protocmp.Transform()); diff != "" {
					t.Errorf("Msg field mismatch (-want, +got):\n%s", diff)
				}
			}
			if diff := cmp.Diff(tt.want.Message, got.Message, protocmp.Transform()); diff != "" {
				t.Errorf("Message field mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestAsProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		msg        *gorums.Message
		wantNil    bool
		wantResult int64
	}{
		{
			name:       "Success",
			msg:        gorums.NewResponseMessage(&gorums.Message{}, dev.Response_builder{Result: 42}.Build()),
			wantNil:    false,
			wantResult: 42,
		},
		{
			name:    "NilMessage",
			msg:     nil,
			wantNil: true,
		},
		{
			name:    "WrongType",
			msg:     gorums.NewResponseMessage(&gorums.Message{}, dev.Request_builder{Value: "request"}.Build()),
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := gorums.AsProto[*dev.Response](tc.msg)
			if tc.wantNil {
				if req != nil {
					t.Errorf("AsProto(%v) returned %v, want nil", tc.msg, req)
				}
				return
			}
			if req == nil {
				t.Errorf("AsProto(%v) returned nil, want *dev.Response", tc.msg)
			}
			if got := req.GetResult(); got != tc.wantResult {
				t.Errorf("GetResult() = %d, want %d", got, tc.wantResult)
			}
		})
	}
}
