package gorums

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestManagerLogging(t *testing.T) {
	var (
		buf    bytes.Buffer
		logger = log.New(&buf, "logger: ", log.Lshortfile)
	)
	mgr := NewManager(InsecureDialOptions(t), WithLogger(logger))
	t.Cleanup(Closer(t, mgr))

	want := "logger: mgr.go:46: ready"
	if strings.TrimSpace(buf.String()) != want {
		t.Errorf("logger: got %q, want %q", buf.String(), want)
	}
}
