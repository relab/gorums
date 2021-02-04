package gorums_test

import (
	"testing"

	"github.com/relab/gorums"
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
