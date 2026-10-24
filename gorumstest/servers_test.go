package gorumstest_test

import (
	"bytes"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mockcall"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
)

// TestServersNilServerFunc verifies that a nil srvFn selects the default server.
func TestServersNilServerFunc(t *testing.T) {
	if got := len(gorumstest.Servers(t, 2, nil)); got != 2 {
		t.Errorf("Servers returned %d addresses, want 2", got)
	}
}

// TestServersEchoServer verifies that EchoServer replies with the request
// value prefixed by "echo: ".
func TestServersEchoServer(t *testing.T) {
	node := gorumstest.Node(t, gorumstest.EchoServer)
	ctx := gorumstest.Context(t, 5*time.Second)
	resp, err := mockcall.RemoteCall(ctx, node, "hello")
	if err != nil {
		t.Fatalf("RemoteCall: %v", err)
	}
	if got, want := resp.GetValue(), "echo: hello"; got != want {
		t.Errorf("RemoteCall = %q, want %q", got, want)
	}
}

// TestLocalServersOptions verifies that LocalServers applies a ServerOption to
// every server, a DialOption to every server's outbound configuration, and a
// LocalServerOption as given.
func TestLocalServersOptions(t *testing.T) {
	const n = 2
	var mu sync.Mutex
	peerChanges := 0
	var perServer []gorums.ID
	var dialLog bytes.Buffer
	gorumstest.LocalServers(t, n,
		gorums.WithPeerChange(func(gorums.Config) {
			mu.Lock()
			defer mu.Unlock()
			peerChanges++
		}),
		gorums.WithLogger(log.New(&dialLog, "", 0)),
		gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
			mu.Lock()
			defer mu.Unlock()
			perServer = append(perServer, id)
			return nil
		}),
	)

	mu.Lock()
	defer mu.Unlock()
	if peerChanges < n {
		t.Errorf("WithPeerChange callback ran %d times; want at least %d, one per server", peerChanges, n)
	}
	if got := strings.Count(dialLog.String(), "ready"); got != n {
		t.Errorf("dial logger recorded %d \"ready\" lines; want %d, one per server", got, n)
	}
	if want := []gorums.ID{1, 2}; !slices.Equal(perServer, want) {
		t.Errorf("WithLocalServerOptionsFunc called for %v; want %v", perServer, want)
	}
}

// TestLocalServersUnsupportedOption verifies that LocalServers panics on an
// Option of a type it cannot apply.
func TestLocalServersUnsupportedOption(t *testing.T) {
	defer func() {
		if got := fmt.Sprint(recover()); !strings.Contains(got, "unsupported LocalServers Option type int") {
			t.Errorf("LocalServers panic = %q; want it to name the unsupported type int", got)
		}
	}()
	gorumstest.LocalServers(t, 1, 42)
}
