package gorums

import "testing"

// TestSplitOptionsTypedNil verifies that splitOptions correctly handles typed
// nils. In Go, an interface can be non-nil while wrapping a nil concrete value
// (e.g. ManagerOption(nil)), which the simple "opt == nil" check does not
// catch. Without a robust check, these typed nils would pass through and cause
// a panic when the caller invokes the nil function.
func TestSplitOptionsTypedNil(t *testing.T) {
	tests := []struct {
		name       string
		opts       []Option
		wantSrvLen int
		wantMgrLen int
	}{
		{
			name:       "UntypedNil",
			opts:       []Option{nil},
			wantSrvLen: 0,
			wantMgrLen: 0,
		},
		{
			name:       "NilManagerOption",
			opts:       []Option{ManagerOption(nil)},
			wantSrvLen: 0,
			wantMgrLen: 0,
		},
		{
			name:       "NilServerOption",
			opts:       []Option{ServerOption(nil)},
			wantSrvLen: 0,
			wantMgrLen: 0,
		},
		{
			name: "MixedNilAndValid",
			opts: []Option{
				ManagerOption(nil),
				WithSendBufferSize(0),
				ServerOption(nil),
				WithReceiveBufferSize(0),
			},
			wantSrvLen: 1,
			wantMgrLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvOpts, mgrOpts, nodeListOpt, err := splitOptions(tt.opts)
			if err != nil {
				t.Fatalf("splitOptions() unexpected error: %v", err)
			}
			if nodeListOpt != nil {
				t.Errorf("nodeListOpt = %v, want nil", nodeListOpt)
			}
			if got := len(srvOpts); got != tt.wantSrvLen {
				t.Errorf("len(srvOpts) = %d, want %d", got, tt.wantSrvLen)
			}
			if got := len(mgrOpts); got != tt.wantMgrLen {
				t.Errorf("len(mgrOpts) = %d, want %d", got, tt.wantMgrLen)
			}
		})
	}
}
