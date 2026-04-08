package gengorums

import (
	"slices"
	"testing"
)

// TestReservedIdentifiers pins the set of identifiers that the bundler will
// mark as reserved and inject as type aliases into every generated _gorums.pb.go
// file. If this test fails, update aliases.go intentionally and then update
// the want slice below to match.
func TestReservedIdentifiers(t *testing.T) {
	pkg := loadPackage("github.com/relab/gorums/cmd/protoc-gen-gorums/dev")
	_, got := findIdentifiers(pkg)
	want := []string{"ConfigContext", "Configuration", "Node", "NodeContext"}
	if !slices.Equal(got, want) {
		t.Errorf("generated static surface changed:\ngot:  %v\nwant: %v\nIf intentional, update aliases.go and this want slice.", got, want)
	}
}
