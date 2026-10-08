package testprotos_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/protoc"
)

// TestFailingProtoFiles verifies that the generator rejects each proto file
// that declares a Gorums reserved identifier, whatever kind of declaration,
// and reports the reason through protoc.
func TestFailingProtoFiles(t *testing.T) {
	tests := []struct {
		name  string
		proto string
	}{
		{name: "Message", proto: "failing/reservednames/reserved.proto"},
		{name: "RPCMethod", proto: "failing/reservedrpc/reserved.proto"},
		{name: "Enum", proto: "failing/reservedenum/reserved.proto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// protoc-gen-go may write its output before the Gorums plugin fails.
			t.Cleanup(func() {
				files, _ := filepath.Glob(filepath.Join(filepath.Dir(tt.proto), "*.pb.go"))
				for _, f := range files {
					_ = os.Remove(f)
				}
			})
			out, err := protoc.Run(tt.proto)
			if err == nil {
				t.Fatalf("expected protoc to fail with:\n%s", out)
			}
			if want := "reserved Gorums identifier"; !strings.Contains(out, want) {
				t.Errorf("protoc output = %q, want it to contain %q", out, want)
			}
		})
	}
}
