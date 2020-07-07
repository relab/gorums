package dev_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	qc "github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
)

func TestIdenticalNodeListsDifferentOrder(t *testing.T) {
	// Equal set of addresses, but different order.
	addrsOne := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	addrsTwo := []string{"localhost:8081", "localhost:8082", "localhost:8080"}

	mgrOne, err := qc.NewManager(qc.WithNoConnect(), qc.WithNodeList(addrsOne))
	if err != nil {
		t.Fatal(err)
	}
	mgrTwo, err := qc.NewManager(qc.WithNoConnect(), qc.WithNodeList(addrsTwo))
	if err != nil {
		t.Fatal(err)
	}

	ids := mgrOne.NodeIDs()

	// Create a configuration in each manager using all nodes.
	// Global ids should be equal.
	configOne, err := mgrOne.NewConfiguration(ids, nil)
	if err != nil {
		t.Fatalf("error creating config one: %v", err)
	}
	configTwo, err := mgrTwo.NewConfiguration(ids, nil)
	if err != nil {
		t.Fatalf("error creating config two: %v", err)
	}
	if configOne.ID() != configTwo.ID() {
		t.Errorf("global configuration ids differ, %d != %d",
			configOne.ID(), configTwo.ID())
	}
}

func TestNodeListDuplicateIDs(t *testing.T) {
	addrs := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	mgr, err := qc.NewManager(qc.WithNoConnect(), qc.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	ids := mgr.NodeIDs()

	// Create a configuration with all ids available in the manager.
	configOne, err := mgr.NewConfiguration(ids, nil)
	if err != nil {
		t.Fatalf("error creating config one: %v", err)
	}
	// Create a configuration with all ids available in the manager and a duplicate.
	configTwo, err := mgr.NewConfiguration(append(ids, ids[0]), nil)
	if err != nil {
		t.Fatalf("error creating config two: %v", err)
	}

	// Global ids should be equal.
	if configOne.ID() != configTwo.ID() {
		t.Errorf("global configuration ids differ, %d != %d",
			configOne.ID(), configTwo.ID())
	}
}

func TestCreateConfiguration(t *testing.T) {
	addrs := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	mgr, err := qc.NewManager(qc.WithNoConnect(), qc.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	ids := mgr.NodeIDs()
	config, err := mgr.NewConfiguration(ids, nil)
	if err != nil {
		t.Fatal(err)
	}

	cfgNodeIDs := config.NodeIDs()
	if diff := cmp.Diff(ids, cfgNodeIDs); diff != "" {
		t.Errorf("NodeIDs() mismatch (-ids +cfgNodeIDs):\n%s", diff)
	}

	_, size := mgr.Size()
	if size != 1 {
		t.Errorf("got #%d configurations from Manager, want %d", size, 1)
	}
}

func TestWithNodeMap(t *testing.T) {
	addrs := map[string]uint32{
		"localhost:8080": 1,
		"localhost:8081": 2,
		"localhost:8082": 3,
		"localhost:8083": 4,
	}

	mgr, err := qc.NewManager(qc.WithNoConnect(), qc.WithNodeMap(addrs))
	if err != nil {
		t.Fatal(err)
	}

	mgrSize, _ := mgr.Size()
	if mgrSize != len(addrs) {
		t.Errorf("Size() = %d; want %d", mgrSize, len(addrs))
	}

	nodes := mgr.NodeIDs()
	if len(nodes) != len(addrs) {
		t.Errorf("Number of nodes %d; want %d", mgrSize, len(addrs))
	}

	// set comparison
	numNodes := 0
	for _, nodeID := range nodes {
		for _, addrID := range addrs {
			if nodeID == addrID {
				numNodes++
			}
		}
	}
	if numNodes != len(addrs) {
		t.Errorf("Found %d unique nodes; want %d", numNodes, len(addrs))
	}
}

func TestWithNodeMapDuplicateIDs(t *testing.T) {
	addrs := map[string]uint32{
		"localhost:8080": 1,
		"localhost:8081": 2,
		"localhost:8082": 3,
		"localhost:8083": 1,
	}

	_, err := qc.NewManager(qc.WithNoConnect(), qc.WithNodeMap(addrs))
	if err == nil {
		t.Errorf("expected error due to duplicate node IDs.")
	}

	addrs["localhost:8083"] = 4

	_, err = qc.NewManager(qc.WithNoConnect(), qc.WithNodeMap(addrs))
	if err != nil {
		t.Fatal(err)
	}
}
