package gorumstest_test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/mockcall"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
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

// errorRecorder is a [testing.TB] that records Errorf instead of failing the
// test, so a test can check that a helper fails the test.
type errorRecorder struct {
	testing.TB
	mu   sync.Mutex
	msgs []string
}

func (r *errorRecorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}

func (r *errorRecorder) messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.msgs)
}

// TestServerLoggerFailsTest verifies that servers from LocalServers, and
// servers built with ServerLogger, fail the test when a handler returns an
// error after send, and that a later WithServerLogger option replaces the
// test logger.
func TestServerLoggerFailsTest(t *testing.T) {
	errorAfterSend := func(_ gorums.ServerContext, _ *gorums.Message, send func(proto.Message)) error {
		send(pb.String("reply"))
		return errors.New("error after send")
	}
	var logs bytes.Buffer
	tests := []struct {
		name       string
		server     func(tb testing.TB) *gorums.Server
		wantFailed bool
	}{
		{
			name: "ServerLogger",
			server: func(tb testing.TB) *gorums.Server {
				srv := gorums.NewServer(gorumstest.ServerLogger(tb))
				t.Cleanup(srv.Stop)
				return srv
			},
			wantFailed: true,
		},
		{
			name:       "LocalServers",
			server:     func(tb testing.TB) *gorums.Server { return gorumstest.LocalServers(tb, 1)[0] },
			wantFailed: true,
		},
		{
			name: "LocalServersWithServerLogger",
			server: func(tb testing.TB) *gorums.Server {
				return gorumstest.LocalServers(tb, 1, gorums.WithServerLogger(log.New(&logs, "", 0)))[0]
			},
			wantFailed: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			rec := &errorRecorder{TB: t}
			srv := tt.server(rec)
			srv.RegisterHandler(mock.TestMethod, errorAfterSend)
			req := stream.Message_builder{Method: mock.TestMethod}.Build()
			srv.HandleRequest(t.Context(), 0, req, func() {}, func(*stream.Message) {})

			msgs := rec.messages()
			if tt.wantFailed {
				if len(msgs) != 1 || !strings.Contains(msgs[0], "programming error") {
					t.Errorf("test failures = %q, want one programming error", msgs)
				}
				return
			}
			if len(msgs) != 0 {
				t.Errorf("test failures = %q, want none", msgs)
			}
			if !strings.Contains(logs.String(), "programming error") {
				t.Errorf("log = %q, want a programming error", logs.String())
			}
		})
	}
}
