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
