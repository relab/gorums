package gorums_test

import (
	"testing"

	"github.com/relab/gorums"
)

func TestNewConfigurationNodeList(t *testing.T) {
	mgr, err := gorums.NewManager(gorums.WithNodeList(nodes), gorums.WithNoConnect())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gorums.NewConfiguration(mgr, mgr.NodeIDs())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Size() != len(nodes) {
		t.Errorf("cfg.Size() = %d, expected %d", cfg.Size(), len(nodes))
	}

	contains := func(nodes []*gorums.Node, addr string) bool {
		for _, node := range nodes {
			if addr == node.Address() {
				return true
			}
		}
		return false
	}
	cfgNodes := cfg.Nodes()
	for _, n := range nodes {
		if !contains(cfgNodes, n) {
			t.Errorf("cfg.Nodes() = %v, expected %s", cfgNodes, n)
		}
	}
}

func TestNewConfigurationNodeMap(t *testing.T) {
	mgr, err := gorums.NewManager(gorums.WithNodeMap(nodeMap), gorums.WithNoConnect())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gorums.NewConfiguration(mgr, mgr.NodeIDs())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Size() != len(nodeMap) {
		t.Errorf("cfg.Size() = %d, expected %d", cfg.Size(), len(nodeMap))
	}
	for _, node := range cfg.Nodes() {
		if nodeMap[node.Address()] != node.ID() {
			t.Errorf("cfg.Nodes()[%s] = %d, expected %d", node.Address(), node.ID(), nodeMap[node.Address()])
		}
	}
}
