package gorumstest_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
)

// TestQuorumCallError verifies that QuorumCallError wraps every node error
// under ErrIncomplete, in ascending node ID order.
func TestQuorumCallError(t *testing.T) {
	err1, err2, err3 := errors.New("e1"), errors.New("e2"), errors.New("e3")
	// Map iteration order is random; repeat to catch a nondeterministic order.
	for range 10 {
		qcErr := gorumstest.QuorumCallError(map[uint32]error{3: err3, 1: err1, 2: err2})
		if !errors.Is(qcErr, gorums.ErrIncomplete) {
			t.Errorf("errors.Is(qcErr, gorums.ErrIncomplete) = false, want true")
		}
		if got, want := qcErr.Unwrap(), []error{err1, err2, err3}; !slices.Equal(got, want) {
			t.Fatalf("qcErr.Unwrap() = %v, want %v", got, want)
		}
	}
	if got := gorumstest.QuorumCallError(nil).NumErrors(); got != 0 {
		t.Errorf("QuorumCallError(nil).NumErrors() = %d, want 0", got)
	}
}
