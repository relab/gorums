package mock

import (
	"slices"
	"testing"
)

// CheckNodeIDs reports a test error if cfg.NodeIDs() is not equal to wantIDs.
// The error message starts with label.
func CheckNodeIDs(t testing.TB, cfg interface{ NodeIDs() []uint32 }, wantIDs []uint32, label string) {
	t.Helper()
	if got := cfg.NodeIDs(); !slices.Equal(got, wantIDs) {
		t.Errorf("%s: config IDs = %v; want %v", label, got, wantIDs)
	}
}
