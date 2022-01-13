package gorums_test

import (
	"bytes"
	"log"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/tests/dummy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	nodes   = []string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}
	nodeMap = map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}
)

type dummyNode struct {
	*gorums.RawNode
}

func (n dummyNode) AsRaw() *gorums.RawNode {
	return n.RawNode
}

func TestManagerLogging(t *testing.T) {
	var (
		buf    bytes.Buffer
		logger = log.New(&buf, "logger: ", log.Lshortfile)
	)
	buf.WriteString("\n")
	_ = gorums.NewRawManager(
		gorums.WithNoConnect(),
		gorums.WithLogger(logger),
	)
	t.Log(buf.String())
}

func TestManagerAddNode(t *testing.T) {
	mgr := gorums.NewRawManager(gorums.WithNoConnect())
	_, err := gorums.NewRawConfiguration[dummyNode](mgr, gorums.WithNodeMap(nodeMap))
	if err != nil {
		t.Fatal(err)
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
		node, err := gorums.NewRawNodeWithID(test.addr, test.id)
		if err != nil {
			t.Fatal(err)
		}
		err = mgr.AddNode(node)
		if err != nil && err.Error() != test.err {
			t.Errorf("mgr.AddNode(Node(%s, %d)) = %s, expected %s", test.addr, test.id, err.Error(), test.err)
		}
	}
}

// Proto definition in tests/dummy/dummy.proto
type dummySrv struct{}

func (_ dummySrv) Test(ctx gorums.ServerCtx, _ *dummy.Empty) (resp *dummy.Empty, err error) {
	return nil, nil
}

func TestManagerAddNodeWithConn(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 3, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		dummy.RegisterDummyServer(srv, &dummySrv{})
		return srv
	})
	defer teardown()
	mgr := gorums.NewRawManager(
		gorums.WithDialTimeout(100*time.Millisecond),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	_, err := gorums.NewRawConfiguration[dummyNode](mgr, gorums.WithNodeList(addrs[:2]))
	if err != nil {
		t.Fatal(err)
	}
	if mgr.Size() != len(addrs)-1 {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(addrs)-1)
	}

	node, err := gorums.NewRawNode(addrs[2])
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.AddNode(node)
	if err != nil {
		t.Errorf("mgr.AddNode(%s) = %q, expected %q", addrs[2], err.Error(), "")
	}
	if mgr.Size() != len(addrs) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(addrs))
	}
}
