package dev_test

import (
	"testing"

	qc "github.com/relab/gorums/dev"
	"github.com/relab/gorums/idutil"
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

	ids := mgrOne.NodeIDs(false)
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

func TestWithSelfAddrOption(t *testing.T) {
	addrs := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	selfAddr := "localhost:8081"
	wantSize := len(addrs)

	mgr, err := qc.NewManager(addrs, qc.WithNoConnect(), qc.WithSelfAddr(selfAddr))
	if err != nil {
		t.Fatal(err)
	}

	gotSize, _ := mgr.Size()
	if gotSize != wantSize {
		t.Errorf("got manager node size %d, want %d", gotSize, wantSize)
	}

	ids := mgr.NodeIDs(false)
	if len(ids) != wantSize {
		t.Errorf("got %d node ids from manager, want %d", len(ids), wantSize)
	}

	nodes := mgr.Nodes(false)
	if len(nodes) != wantSize {
		t.Errorf("got %d nodes from manager, want %d", len(nodes), wantSize)
	}

	nodes = mgr.Nodes(true)
	if len(nodes) != wantSize-1 {
		t.Errorf("got %d nodes from manager, want %d", len(nodes), wantSize-1)
	}

	notPresentAddr := "localhost:8083"
	_, err = qc.NewManager(addrs, qc.WithNoConnect(), qc.WithSelfAddr(notPresentAddr))
	if err == nil {
		t.Error("got no manager creation error, want error due to invaild WithSelf option")
	}
}

func TestWithSelfGidOption(t *testing.T) {
	addrs := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	selfAddr := "localhost:8081"
	selfID, err := idutil.IDFromAddress(selfAddr)
	if err != nil {
		t.Fatal(err)
	}
	wantSize := len(addrs)

	mgr, err := qc.NewManager(addrs, qc.WithNoConnect(), qc.WithSelfID(selfID))
	if err != nil {
		t.Fatal(err)
	}

	gotSize, _ := mgr.Size()
	if gotSize != wantSize {
		t.Errorf("got manager node size %d, want %d", gotSize, wantSize)
	}

	ids := mgr.NodeIDs(false)
	if len(ids) != wantSize {
		t.Errorf("got %d node ids from manager, want %d", len(ids), wantSize)
	}

	nodes := mgr.Nodes(false)
	if len(nodes) != wantSize {
		t.Errorf("got %d nodes from manager, want %d", len(nodes), wantSize)
	}

	nodes = mgr.Nodes(true)
	if len(nodes) != wantSize-1 {
		t.Errorf("got %d nodes from manager, want %d", len(nodes), wantSize-1)
	}

	var notPresentID uint32 = 42
	_, err = qc.NewManager(addrs, qc.WithNoConnect(), qc.WithSelfID(notPresentID))
	if err == nil {
		t.Error("got no manager creation error, want error due to invaild WithSelfID option")
	}
}

func TestCreateConfiguration(t *testing.T) {
	addrs := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	mgr, err := qc.NewManager(addrs, qc.WithNoConnect())
	if err != nil {
		t.Fatal(err)
	}

	ids := mgr.NodeIDs(false)
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

func TestCreateConfiguratonWithSelfOption(t *testing.T) {
	addrs := []string{"localhost:8080", "localhost:8081", "localhost:8082"}
	selfAddr := "localhost:8081"
	selfID, err := idutil.IDFromAddress(selfAddr)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := qc.NewManager(addrs, qc.WithNoConnect(), qc.WithSelfID(selfID))
	if err != nil {
		t.Fatal(err)
	}

	ids := mgr.NodeIDs(false)
	qspecOne := NewMajorityQSpec(len(ids))

	_, err = mgr.NewConfiguration(ids, qspecOne)
	if err == nil {
		t.Error("expected error creating configuration with self, got none")
	}

	nodes := mgr.Nodes(true)
	var nids []uint32
	for _, node := range nodes {
		nids = append(nids, node.ID())
	}

	qspecTwo := NewMajorityQSpec(len(nids))

	config, err := mgr.NewConfiguration(nids, qspecTwo)
	if err != nil {
		t.Errorf("got error creating configuration, want none (%v)", err)
	}

	cids := config.NodeIDs()
	if !equal(cids, nids) {
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
