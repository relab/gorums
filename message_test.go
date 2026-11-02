package gorums_test

import (
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
)

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
			msg:        &gorums.Message{Proto: dev.Response_builder{Result: 42}.Build()},
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
			msg:     &gorums.Message{Proto: dev.Request_builder{Value: "request"}.Build()},
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
