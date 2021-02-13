package gorums_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums"
)

func TestNewConfigurationNodeList(t *testing.T) {
	mgr := gorums.NewManager(gorums.WithNoConnect())
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
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

	if mgr.Size() != len(nodes) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodes))
	}
	mgrNodes := cfg.Nodes()
	for _, n := range nodes {
		if !contains(mgrNodes, n) {
			t.Errorf("mgr.Nodes() = %v, expected %s", mgrNodes, n)
		}
	}
}

func TestNewConfigurationNodeMap(t *testing.T) {
	mgr := gorums.NewManager(gorums.WithNoConnect())
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeMap(nodeMap))
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
	if mgr.Size() != len(nodeMap) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodeMap))
	}
	for _, node := range mgr.Nodes() {
		if nodeMap[node.Address()] != node.ID() {
			t.Errorf("mgr.Nodes()[%s] = %d, expected %d", node.Address(), node.ID(), nodeMap[node.Address()])
		}
	}
}

func TestNewConfigurationNodeIDs(t *testing.T) {
	mgr := gorums.NewManager(gorums.WithNoConnect())
	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	if c1.Size() != len(nodes) {
		t.Errorf("c1.Size() = %d, expected %d", c1.Size(), len(nodes))
	}

	// Identical configurations c1 == c2
	nodeIDs := c1.NodeIDs()
	c2, err := gorums.NewConfiguration(mgr, gorums.WithNodeIDs(nodeIDs))
	if err != nil {
		t.Fatal(err)
	}
	if c2.Size() != len(nodes) {
		t.Errorf("c2.Size() = %d, expected %d", c2.Size(), len(nodes))
	}
	if diff := cmp.Diff(c1, c2); diff != "" {
		t.Errorf("Expected same configurations, but got (-c1 +c2):\n%s", diff)
	}

	// Configuration with one less node |c3| == |c1| - 1
	c3, err := gorums.NewConfiguration(mgr, gorums.WithNodeIDs(nodeIDs[:len(nodeIDs)-1]))
	if err != nil {
		t.Fatal(err)
	}
	if c3.Size() != len(nodes)-1 {
		t.Errorf("c3.Size() = %d, expected %d", c3.Size(), len(nodes)-1)
	}
	if diff := cmp.Diff(c1, c3); diff == "" {
		t.Errorf("Expected different configurations, but got not difference")
	}
}
