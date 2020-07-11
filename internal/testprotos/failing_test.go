package testprotos_test

import (
	"testing"

	"github.com/relab/gorums/internal/protoc"
)

func TestFailingProtoFiles(t *testing.T) {
	out, err := protoc.Run("sourceRelative", "failing/reservednames/reserved.proto")
	if err == nil {
		t.Fatalf("expected protoc to fail with:\n%s", out)
	}
}
