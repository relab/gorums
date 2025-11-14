package gorums

import (
	"testing"

	"github.com/relab/gorums/internal/tests/mock"
)

func TestAsProto(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		wantNil bool
		wantVal string
	}{
		{
			name:    "Success",
			msg:     newRequestMessage(nil, mock.Request_builder{Val: "hello"}.Build()),
			wantNil: false,
			wantVal: "hello",
		},
		{
			name:    "NilMessage",
			msg:     nil,
			wantNil: true,
		},
		{
			name:    "WrongType",
			msg:     NewResponseMessage(nil, mock.Response_builder{Val: "r"}.Build()),
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := AsProto[*mock.Request](tc.msg)
			if tc.wantNil {
				if req != nil {
					t.Errorf("AsProto returned %v, want nil", req)
				}
				return
			}
			if req == nil {
				t.Errorf("AsProto returned nil, want *mock.Request")
			}
			if got := req.GetVal(); got != tc.wantVal {
				t.Errorf("Val = %q, want %q", got, tc.wantVal)
			}
		})
	}
}
