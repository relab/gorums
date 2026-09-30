package gorums_test

import (
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
)

// TestLocalServersMissingDialOptions verifies that NewLocalServers reports an
// invalid peer configuration as an error, and returns no servers to stop.
func TestLocalServersMissingDialOptions(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(4)
	if err == nil {
		if stop != nil {
			stop()
		}
		t.Fatal("NewLocalServers without dial options succeeded, want an error")
	}
	if servers != nil || stop != nil {
		t.Errorf("NewLocalServers returned servers=%v stop=%v with an error", servers, stop != nil)
	}
}

// TestLocalServersStopClosesPreallocatedListener verifies that the stop
// function closes a server's preallocated listener even when the server
// was started on a different listener with Serve.
func TestLocalServersStopClosesPreallocatedListener(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(2, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatal(err)
	}
	preallocated := servers[0].Addr()
	// The test serves on its own listener, which is what the server must not
	// confuse with the preallocated one.
	own, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = servers[0].Serve(own) }()
	go func() { _ = servers[1].ListenAndServe() }()
	gorumstest.WaitUntil(t, 2*time.Second, func() bool { return servers[0].Addr() == own.Addr().String() })

	stop()
	if c, err := net.DialTimeout("tcp", preallocated, time.Second); err == nil {
		_ = c.Close()
		t.Errorf("preallocated listener %s still accepts connections after stop", preallocated)
	}
}
