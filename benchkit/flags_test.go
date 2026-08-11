package benchkit

import (
	"flag"
	"testing"
	"time"
)

// TestRegisterFlagsCallTimeout verifies that -call-timeout is parsed into
// StandardFlags and carried into Options, and that it defaults to disabled.
func TestRegisterFlagsCallTimeout(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want time.Duration
	}{
		{name: "DefaultDisabled", args: nil, want: 0},
		{name: "Set", args: []string{"-call-timeout=2s"}, want: 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
			f := RegisterFlags(fs)
			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("Parse(%v) failed: %v", tt.args, err)
			}
			if got := f.Options().CallTimeout; got != tt.want {
				t.Errorf("Options().CallTimeout = %v, want %v", got, tt.want)
			}
		})
	}
}
