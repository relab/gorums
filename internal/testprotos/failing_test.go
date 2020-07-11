package testprotos_test

import (
	"testing"

	"github.com/relab/gorums/internal/protoc"
)

func TestFailingProtoFiles(t *testing.T) {
	err := protoc.Run("sourceRelative", "failing/reservednames/reserved.proto")
	if err == nil {
		t.Fatal("expected protoc to fail with '--gorums_out: protoc-gen-gorums: Plugin failed with status code 1.'")
	}
}
