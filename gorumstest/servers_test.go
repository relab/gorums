package gorumstest_test

import (
	"testing"

	"github.com/relab/gorums/gorumstest"
)

// TestServersNilServerFunc verifies that a nil srvFn selects the default server.
func TestServersNilServerFunc(t *testing.T) {
	if got := len(gorumstest.Servers(t, 2, nil)); got != 2 {
		t.Errorf("Servers returned %d addresses, want 2", got)
	}
}
