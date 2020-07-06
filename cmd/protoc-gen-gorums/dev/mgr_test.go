package dev_test

import (
	"testing"

	qc "github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
)

func TestEqualGlobalConfigurationIDsDifferentOrder(t *testing.T) {
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

func TestEqualGlobalConfigurationIDsDuplicateID(t *testing.T) {
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
		t.Errorf("got error creating configuration, want none (%v)", err)
	}

	cfgNodeIDs := config.NodeIDs()
	if !equal(cfgNodeIDs, ids) {
		t.Errorf("ids from Manager (got %v) and ids from configuration containing all nodes (got %v) should be equal",
			ids, cfgNodeIDs)
	}

	_, size := mgr.Size()
	if size != 1 {
		t.Errorf("got #%d configurations from Manager, want %d", size, 1)
	}
}

func equal(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if x != b[i] {
			return false
		}
	}
	return true
}

func TestSortedNodesInMappedGlobalConfiguration(t *testing.T) {
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

	ids := mgr.NodeIDs()
	amountOfNodes := 0
	for i, id := range ids {
		if i+1 != int(id) {
			t.Errorf("node ids are not created in the correct order. Expected %d, got %d", i+1, int(id))
		}
		amountOfNodes++
	}

	if amountOfNodes != 4 {
		t.Errorf("amount of nodes created is not correct. Expected 4 nodes, got %d", amountOfNodes)
	}

}

func TestIDduplicationInMappedGlobalConfiguration(t *testing.T) {
	addrs := map[string]uint32{
		"localhost:8080": 1,
		"localhost:8081": 2,
		"localhost:8082": 3,
		"localhost:8083": 1,
	}

	_, err1 := qc.NewManager(qc.WithNoConnect(), qc.WithNodeMap(addrs))
	if err1 == nil {
		t.Errorf("expected error. No error given when there are duplicate node ids.")
	}

	addrs["localhost:8083"] = 4

	_, err2 := qc.NewManager(qc.WithNoConnect(), qc.WithNodeMap(addrs))
	if err2 != nil {
		t.Fatal(err2)
	}
}
