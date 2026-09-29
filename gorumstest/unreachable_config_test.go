package gorumstest_test

import (
	"net"
	"strconv"
	"testing"

	"github.com/relab/gorums/gorumstest"
)

// linuxEphemeralStart is the lower bound of the default ephemeral port range
// on Linux. The macOS and IANA ranges start higher, at 49152, so a port below
// this value is outside every one of those ranges.
const linuxEphemeralStart = 32768

func TestUnreachableConfigSentinelOutsideEphemeralRange(t *testing.T) {
	cfg := gorumstest.UnreachableConfig(t)
	nodes := cfg.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("Nodes() = %d nodes, want 1", len(nodes))
	}
	host, portStr, err := net.SplitHostPort(nodes[0].Address())
	if err != nil {
		t.Fatalf("Address() = %q: %v", nodes[0].Address(), err)
	}
	if host != "127.0.0.1" {
		t.Errorf("sentinel host = %q, want 127.0.0.1", host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("sentinel port %q: %v", portStr, err)
	}
	if port == 0 || port >= linuxEphemeralStart {
		t.Errorf("sentinel port %d is inside an ephemeral range", port)
	}
}
