package gorumstest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/relab/gorums/gorumstest"
)

// TestOptionsUnknownTypePanics verifies that Config panics with a message
// naming the type of an Option value it does not recognize, instead of
// silently ignoring it.
func TestOptionsUnknownTypePanics(t *testing.T) {
	tests := []struct {
		opt      gorumstest.Option
		wantType string
	}{
		{opt: 42, wantType: "int"},
		{opt: "WithFoo", wantType: "string"},
		{opt: nil, wantType: "<nil>"},
	}
	for _, tt := range tests {
		t.Run(tt.wantType, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("Config did not panic on an unknown Option type")
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, tt.wantType) {
					t.Errorf("panic message %q does not name type %q", msg, tt.wantType)
				}
			}()
			gorumstest.Config(t, 1, nil, tt.opt)
		})
	}
}
