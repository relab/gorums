package gorums_test

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/tests/unresponsive"
	"google.golang.org/grpc"
)

func TestNewManager(t *testing.T) {
	_, err := gorums.NewManager()
	if err == nil {
		t.Errorf("NewManager(): expected error: '%s'", "could not create manager: no nodes provided")
	}

	nodes := []string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}
	mgr, err := gorums.NewManager(gorums.WithNodeList(nodes), gorums.WithNoConnect())
	if err != nil {
		t.Fatalf("NewManager(): unexpected error: %s", err)
	}
	if mgr.Size() != len(nodes) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodes))
	}

	nodeMap := map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}
	mgr, err = gorums.NewManager(gorums.WithNodeMap(nodeMap), gorums.WithNoConnect())
	if err != nil {
		t.Fatalf("NewManager(): unexpected error: %s", err)
	}
	if mgr.Size() != len(nodeMap) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodeMap))
	}
}

func TestManagerLogging(t *testing.T) {
	var (
		buf    bytes.Buffer
		logger = log.New(&buf, "logger: ", log.Lshortfile)
	)
	buf.WriteString("\n")
	nodeMap := map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}
	mgr, err := gorums.NewManager(
		gorums.WithNodeMap(nodeMap),
		gorums.WithNoConnect(),
		gorums.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewManager(): unexpected error: %s", err)
	}
	if mgr.Size() != len(nodeMap) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodeMap))
	}
	t.Log(buf.String())
}

func TestManagerAddNode(t *testing.T) {
	nodeMap := map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}
	mgr, err := gorums.NewManager(gorums.WithNodeMap(nodeMap), gorums.WithNoConnect())
	if err != nil {
		t.Fatalf("NewManager(): unexpected error: %s", err)
	}
	tests := []struct {
		addr string
		id   uint32
		err  string
	}{
		{"127.0.1.1:1234", 1, "node ID 1 already exists (127.0.1.1:1234)"},
		{"127.0.1.1:1234", 5, ""},
		{"127.0.1.1:1234", 6, ""}, // TODO(meling) does it make sense to allow same addr:port for different IDs?
		{"127.0.1.1:1234", 2, "node ID 2 already exists (127.0.1.1:1234)"},
	}
	for _, test := range tests {
		err = mgr.AddNode(test.addr, test.id)
		if err != nil && err.Error() != test.err {
			t.Errorf("mgr.AddNode(%s, %d) = %s, expected %s", test.addr, test.id, err.Error(), test.err)
		}
	}
}

// Reusing tests/unresponsive/unresponsive.proto definitions
type testSrv struct{}

func (srv testSrv) TestUnresponsive(ctx context.Context, _ *unresponsive.Empty, _ func(*unresponsive.Empty, error)) {
}

func TestManagerAddNodeWithConn(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 3, func() gorums.ServerIface {
		srv := gorums.NewServer()
		unresponsive.RegisterUnresponsiveServer(srv, &testSrv{})
		return srv
	})
	defer teardown()
	mgr, err := gorums.NewManager(
		gorums.WithNodeList(addrs[:2]),
		gorums.WithDialTimeout(100*time.Millisecond),
		gorums.WithGrpcDialOptions(grpc.WithInsecure(), grpc.WithBlock()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.Size() != len(addrs)-1 {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(addrs)-1)
	}

	err = mgr.AddNode(addrs[2], 1)
	if err != nil {
		t.Errorf("mgr.AddNode(%s, %d) = %q, expected %q", addrs[2], 0, err.Error(), "")
	}
	if mgr.Size() != len(addrs) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(addrs))
	}
}
