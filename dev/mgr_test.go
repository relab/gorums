package dev_test

import (
	"testing"

	qc "github.com/relab/gorums/dev"
)

func TestEqualGlobalConfigurationIDs(t *testing.T) {
	// Equal set of addresses, but different order.
	addrsOne := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	addrsTwo := []string{"localhost:8081", "localhost:8082", "localhost:8080"}

	mgrOne, err := qc.NewManager(addrsOne, qc.WithNoConnect())
	if err != nil {
		t.Fatal(err)
	}
	mgrTwo, err := qc.NewManager(addrsTwo, qc.WithNoConnect())
	if err != nil {
		t.Fatal(err)
	}

	ids := mgrOne.NodeIDs()
	qspec := NewMajorityQSpec(len(ids))

	// Create a configuration in each manager using all nodes.
	// Global ids should be equal.
	configOne, err := mgrOne.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config one: %v", err)
	}
	configTwo, err := mgrTwo.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config two: %v", err)
	}
	if configOne.ID() != configTwo.ID() {
		t.Errorf("global configuration ids differ, %d != %d",
			configOne.ID(), configTwo.ID())
	}
}

func TestCreateConfiguration(t *testing.T) {
	addrs := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	mgr, err := qc.NewManager(addrs, qc.WithNoConnect())
	if err != nil {
		t.Fatal(err)
	}

	ids := mgr.NodeIDs()
	qspec := NewMajorityQSpec(len(ids))

	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Errorf("got error creating configuration, want none (%v)", err)
	}

	cids := config.NodeIDs()
	if !equal(cids, ids) {
		t.Errorf("ids from Manager (got %v) and ids from configuration containing all nodes (got %v) should be equal",
			ids, cids)
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
