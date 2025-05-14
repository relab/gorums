package gorums

import (
	"bytes"
	"log"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var mgrTestNodeMap = map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}

func TestManagerLogging(t *testing.T) {
	var (
		buf    bytes.Buffer
		logger = log.New(&buf, "logger: ", log.Lshortfile)
	)
	buf.WriteString("\n")
	_ = newManager(
		WithNoConnect(),
		WithLogger(logger),
	)
	t.Log(buf.String())
}

func TestManagerAddNode(t *testing.T) {
	cfg, err := NewConfiguration(WithNodeMap(mgrTestNodeMap), WithNoConnect())
	mgr := cfg.mgr
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		addr string
		id   uint32
		err  string
	}{
		{"127.0.1.1:1234", 1, "config: node 1 (127.0.1.1:1234) already exists"},
		{"127.0.1.1:1234", 5, ""},
		{"127.0.1.1:1234", 6, ""}, // The same addr:port can have different IDs
		{"127.0.1.1:1234", 2, "config: node 2 (127.0.1.1:1234) already exists"},
	}
	for _, test := range tests {
		node, err := NewNodeWithID(test.addr, test.id)
		if err != nil {
			t.Fatal(err)
		}
		err = mgr.addNode(node)
		if err != nil && err.Error() != test.err {
			t.Errorf("mgr.addNode(Node(%s, %d)) = %s, expected %s", test.addr, test.id, err.Error(), test.err)
		}
	}
}

func TestManagerAddNodeWithConn(t *testing.T) {
	addrs, teardown := TestSetup(t, 3, func(_ int) ServerIface {
		srv := NewServer()
		return srv
	})
	defer teardown()

	cfg, err := NewConfiguration(WithNodeList(addrs[:2]),
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Close()

	mgr := cfg.mgr

	if mgr.size() != len(addrs)-1 {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.size(), len(addrs)-1)
	}

	node, err := NewNode(addrs[2])
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.addNode(node)
	if err != nil {
		t.Errorf("mgr.AddNode(%s) = %q, expected %q", addrs[2], err.Error(), "")
	}
	if mgr.size() != len(addrs) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.size(), len(addrs))
	}
}
