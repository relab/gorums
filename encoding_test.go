package gorums_test

import (
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/tests/config"
)

func TestAsProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     *gorums.Message
		wantNil bool
		wantNum uint64
	}{
		{
			name:    "Success",
			msg:     gorums.NewResponseMessage(nil, config.Response_builder{Name: "test", Num: 42}.Build()),
			wantNil: false,
			wantNum: 42,
		},
		{
			name:    "NilMessage",
			msg:     nil,
			wantNil: true,
		},
		{
			name:    "WrongType",
			msg:     gorums.NewResponseMessage(nil, config.Request_builder{Num: 99}.Build()),
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := gorums.AsProto[*config.Response](tc.msg)
			if tc.wantNil {
				if req != nil {
					t.Errorf("AsProto returned %v, want nil", req)
				}
				return
			}
			if req == nil {
				t.Errorf("AsProto returned nil, want *config.Response")
			}
			if got := req.GetNum(); got != tc.wantNum {
				t.Errorf("Num = %d, want %d", got, tc.wantNum)
			}
		})
	}
}
