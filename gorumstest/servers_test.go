package gorumstest_test

import (
	"testing"
	"time"

	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
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
	resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](node.Context(ctx), pb.String("hello"), mock.TestMethod)
	if err != nil {
		t.Fatalf("RemoteCall: %v", err)
	}
	if got, want := resp.GetValue(), "echo: hello"; got != want {
		t.Errorf("RemoteCall = %q, want %q", got, want)
	}
}
