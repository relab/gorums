package gorums

import "testing"

// TestSplitOptionsTypedNil verifies that splitOptions correctly handles typed
// nils. In Go, an interface can be non-nil while wrapping a nil concrete value
// (e.g. DialOption(nil)), which the simple "opt == nil" check does not
// catch. Without a robust check, these typed nils would pass through and cause
// a panic when the caller invokes the nil function.
func TestSplitOptionsTypedNil(t *testing.T) {
	tests := []struct {
		name       string
		opts       []Option
		wantSrvLen int
		wantDialLen int
	}{
		{
			name:       "UntypedNil",
			opts:       []Option{nil},
			wantSrvLen: 0,
			wantDialLen: 0,
		},
		{
			name:       "NilDialOption",
			opts:       []Option{DialOption(nil)},
			wantSrvLen: 0,
			wantDialLen: 0,
		},
		{
			name:       "NilServerOption",
			opts:       []Option{ServerOption(nil)},
			wantSrvLen: 0,
			wantDialLen: 0,
		},
		{
			name: "MixedNilAndValid",
			opts: []Option{
				DialOption(nil),
				WithSendBufferSize(0),
				ServerOption(nil),
				WithReceiveBufferSize(0),
			},
			wantSrvLen: 1,
			wantDialLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvOpts, dialOpts, nodeListOpt, err := splitOptions(tt.opts)
			if err != nil {
				t.Fatalf("splitOptions() unexpected error: %v", err)
			}
			if nodeListOpt != nil {
				t.Errorf("nodeListOpt = %v, want nil", nodeListOpt)
			}
			if got := len(srvOpts); got != tt.wantSrvLen {
				t.Errorf("len(srvOpts) = %d, want %d", got, tt.wantSrvLen)
			}
			if got := len(dialOpts); got != tt.wantDialLen {
				t.Errorf("len(dialOpts) = %d, want %d", got, tt.wantDialLen)
			}
		})
	}
}
