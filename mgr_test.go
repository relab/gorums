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

	got := strings.TrimSpace(buf.String())
	wantPrefix := "logger: mgr.go:"
	wantSuffix := ": ready"
	if !strings.HasPrefix(got, wantPrefix) || !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("logger: got %q, want %q<line>%q", got, wantPrefix, wantSuffix)
	}
}
