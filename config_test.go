package gorums_test

import (
	"errors"
	"testing"

	"github.com/relab/gorums"
	"google.golang.org/grpc/encoding"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

var (
	nodes   = []string{"127.0.0.1:9081", "127.0.0.1:9082", "127.0.0.1:9083"}
	nodeMap = map[uint32]testNode{
		1: {addr: "127.0.0.1:9081"},
		2: {addr: "127.0.0.1:9082"},
		3: {addr: "127.0.0.1:9083"},
		4: {addr: "127.0.0.1:9084"},
	}
)

type testNode struct {
	addr string
}

func (n testNode) Addr() string {
	return n.addr
}

func TestNewConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		opt      gorums.NodeListOption
		wantSize int
		wantErr  string
	}{
		{
			name:     "WithNodeList/Success",
			opt:      gorums.WithNodeList(nodes),
			wantSize: len(nodes),
		},
		{
			name:     "WithNodes/Success",
			opt:      gorums.WithNodes(nodeMap),
			wantSize: len(nodeMap),
		},
		{
			name:    "WithNodeList/Reject/EmptyNodeList",
			opt:     gorums.WithNodeList([]string{}),
			wantErr: "config: missing required node addresses",
		},
		{
			name:    "WithNodes/Reject/EmptyNodeMap",
			opt:     gorums.WithNodes(map[uint32]testNode{}),
			wantErr: "config: missing required node map",
		},
		{
			name: "WithNodes/Reject/ZeroID",
			opt: gorums.WithNodes(map[uint32]testNode{
				0: {addr: "127.0.0.1:9080"}, // ID 0 should be rejected
				1: {addr: "127.0.0.1:9081"},
			}),
			wantErr: "config: node 0 is reserved",
		},
		{
			name: "WithNodes/Reject/DuplicateAddress",
			opt: gorums.WithNodes(map[uint32]testNode{
				1: {addr: "127.0.0.1:9081"},
				2: {addr: "127.0.0.1:9081"}, // Duplicate address
			}),
			wantErr: `config: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodeList/Reject/DuplicateAddress",
			opt: gorums.WithNodeList([]string{
				"127.0.0.1:9081",
				"127.0.0.1:9081", // Duplicate address
			}),
			wantErr: `config: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodes/Reject/NormalizedDuplicateAddress",
			opt: gorums.WithNodes(map[uint32]testNode{
				1: {addr: "localhost:9081"},
				2: {addr: "127.0.0.1:9081"}, // Same resolved address
			}),
			wantErr: `config: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodeList/Reject/NormalizedDuplicateAddress",
			opt: gorums.WithNodeList([]string{
				"localhost:9081",
				"127.0.0.1:9081", // Same resolved address
			}),
			wantErr: `config: address "127.0.0.1:9081" already in use by node 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
			t.Cleanup(gorums.Closer(t, mgr))

			cfg, err := gorums.NewConfiguration(mgr, tt.opt)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("Error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Size() != tt.wantSize {
				t.Errorf("cfg.Size() = %d, want %d", cfg.Size(), tt.wantSize)
			}
			if mgr.Size() != tt.wantSize {
				t.Errorf("mgr.Size() = %d, want %d", mgr.Size(), tt.wantSize)
			}
		})
	}
}

func TestConfigurationExtend(t *testing.T) {
	nodes12 := nodes[:2] // {1,2}

	tests := []struct {
		name         string
		initialNodes []string
		extendOpt    gorums.NodeListOption
		wantSize     int
		wantErr      string
	}{
		{
			name:         "WithNil/Success",
			initialNodes: nodes12,
			extendOpt:    nil,
			wantSize:     2,
		},
		{
			name:         "WithNodeList/Success",
			initialNodes: nodes12,
			extendOpt:    gorums.WithNodeList(nodes[2:]),
			wantSize:     len(nodes),
		},
		{
			name:         "WithNodes/Success",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				10: {addr: "127.0.0.1:9090"},
				11: {addr: "127.0.0.1:9091"},
			}),
			wantSize: 4, // 2 initial + 2 new
		},
		{
			name:         "WithNodes/Reject/ZeroID",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				0: {addr: "127.0.0.1:9090"}, // ID 0 should be rejected
			}),
			wantErr: "config: node 0 is reserved",
		},
		{
			name:         "WithNodes/Reject/IDConflict",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				2: {addr: "127.0.0.1:9090"}, // ID 2 already exists, rejected
			}),
			wantErr: "config: node 2 already in use",
		},
		{
			name:         "WithNodes/Reject/AddressConflict",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				3: {addr: "127.0.0.1:9081"}, // Same address as ID 1
			}),
			wantErr: `config: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name:         "WithNodes/Reject/NormalizedAddressConflict",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				3: {addr: "localhost:9081"}, // Resolves to same as existing node 1
			}),
			wantErr: `config: address "127.0.0.1:9081" already in use by node 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
			t.Cleanup(gorums.Closer(t, mgr))

			c, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(tt.initialNodes))
			if err != nil {
				t.Fatal(err)
			}

			c2, err := c.Extend(tt.extendOpt)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("Error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c2.Size() != tt.wantSize {
				t.Errorf("c2.Size() = %d, want %d", c2.Size(), tt.wantSize)
			}
			if mgr.Size() != tt.wantSize {
				t.Errorf("mgr.Size() = %d, want %d", mgr.Size(), tt.wantSize)
			}
		})
	}
}

func TestConfigurationAdd(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	if c1.Size() != len(nodes) {
		t.Errorf("c1.Size() = %d, want %d", c1.Size(), len(nodes))
	}

	// Add existing c1 IDs (should return same configuration)
	nodeIDs := c1.NodeIDs()
	c2 := c1.Add(nodeIDs...) // c2 = {1, 2, 3}
	if c2.Size() != len(nodes) {
		t.Errorf("c2.Size() = %d, want %d", c2.Size(), len(nodes))
	}
	if !c1.Equal(c2) {
		t.Errorf("c1.Equal(c2) = false, want true")
	}

	// Add non-existent ID (should be ignored)
	c3 := c1.Add(999) // c3 = {1, 2, 3}
	if c3.Size() != len(nodes) {
		t.Errorf("c3.Size() = %d, want %d", c3.Size(), len(nodes))
	}
}

func TestConfigurationUnion(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}

	// Add newNodes to c1 using Extend (gets IDs 4, 5)
	newNodes := []string{"127.0.0.1:9084", "127.0.0.1:9085"}
	c3, err := c1.Extend(gorums.WithNodeList(newNodes)) // c3 = {1, 2, 3, 4, 5}
	if err != nil {
		t.Fatal(err)
	}
	if c3.Size() != len(nodes)+len(newNodes) {
		t.Errorf("c3.Size() = %d, want %d", c3.Size(), len(nodes)+len(newNodes))
	}

	// Create c2 as a subset of c1 (first two nodes)
	c2 := c1.Remove(c1[2].ID()) // c2 = {1, 2}
	if c2.Size() != 2 {
		t.Errorf("c2.Size() = %d, want 2", c2.Size())
	}

	// Union c1 with c2 should equal c1 (since c2 is a subset)
	c4 := c1.Union(c2) // c4 = {1, 2, 3}
	if c4.Size() != c1.Size() {
		t.Errorf("c4.Size() = %d, want %d", c4.Size(), c1.Size())
	}
	if !c1.Equal(c4) {
		t.Errorf("c1.Equal(c4) = false, want true")
	}

	// Union c2 with c3 should include all 5 nodes
	c5 := c2.Union(c3)          // c5 = {1, 2, 3, 4, 5}
	if c5.Size() != c3.Size() { // c3 already has IDs 1,2,3,4,5
		t.Errorf("c5.Size() = %d, want %d", c5.Size(), c3.Size())
	}
	if !c5.Equal(c3) {
		t.Errorf("c5.Equal(c3) = false, want true")
	}
}

func TestConfigurationRemove(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}

	// Remove one node using Remove
	c2 := c1.Remove(c1[0].ID()) // c2 = {2, 3}
	if c2.Size() != c1.Size()-1 {
		t.Errorf("c2.Size() = %d, want %d", c2.Size(), c1.Size()-1)
	}
}

func TestConfigurationDifference(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}

	newNodes := []string{"127.0.0.1:9084", "127.0.0.1:9085"}
	c3, err := c1.Extend(gorums.WithNodeList(newNodes)) // c3 = {1, 2, 3, 4, 5}
	if err != nil {
		t.Fatal(err)
	}

	// c4 = c3 - c1 (should be just the new nodes)
	c4 := c3.Difference(c1) // c4 = {4, 5}
	if c4.Size() != c3.Size()-c1.Size() {
		t.Errorf("c4.Size() = %d, want %d", c4.Size(), c3.Size()-c1.Size())
	}
}

func TestConfigurationAddDuplicateIDs(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	// Create configuration with all three nodes
	c2, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c2 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	// Create c1 by removing node 3
	c1 := c2.Remove(3) // c1 = {1, 2}

	// Test Add with the same ID passed multiple times
	// c1 = {1, 2}, we add ID 3 three times - should result in {1, 2, 3} not {1, 2, 3, 3, 3}
	c3 := c1.Add(3, 3, 3)
	if c3.Size() != 3 {
		t.Errorf("c3.Size() = %d, want 3 (duplicates should be ignored)", c3.Size())
	}

	// Verify c2 and c3 have the same IDs
	if !c2.Equal(c3) {
		t.Errorf("c2.Equal(c3) = false, want true")
	}
}

func TestConfigurationUnionDuplicateNodes(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	// Create configuration with nodes 1, 2, 3
	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}

	// Create subset configurations
	c2 := c1.Remove(2, 3) // c2 = {1}
	c3 := c1.Remove(2, 3) // c3 = {1}

	// Union c2 with c3 - since both contain only node 1, result should be {1}
	c4 := c2.Union(c3)
	if c4.Size() != 1 {
		t.Errorf("c4.Size() = %d, want 1", c4.Size())
	}

	// Test Union with overlapping nodes
	c5 := c1.Remove(3) // c5 = {1, 2}
	c6 := c1.Remove(1) // c6 = {2, 3}

	// Union should give {1, 2, 3} - no duplicates
	c7 := c5.Union(c6)
	if c7.Size() != 3 {
		t.Errorf("c7.Size() = %d, want 3", c7.Size())
	}
}

func TestConfigurationImmutability(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}

	// Test Union with empty returns a clone, not the original
	var emptyConfig gorums.Configuration
	c2 := c1.Union(emptyConfig)
	if !c1.Equal(c2) {
		t.Errorf("c1.Equal(c2) = false, want true")
	}

	// Check that the slices don't share the same backing array
	c1Slice := c1.Nodes()
	c2Slice := c2.Nodes()
	if &c1Slice[0] == &c2Slice[0] {
		t.Error("Union(empty) returns same backing array - violates immutability")
	}

	// Test Extend with nil opt returns a clone, not the original
	c3, err := c1.Extend(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !c1.Equal(c3) {
		t.Errorf("c1.Equal(c3) = false, want true")
	}

	// Check that the slices don't share the same backing array
	c3Slice := c3.Nodes()
	if &c1Slice[0] == &c3Slice[0] {
		t.Error("Extend(nil) returns same backing array - violates immutability")
	}
}

func TestConfigurationWithoutErrors(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodes(nodeMap))
	if err != nil {
		t.Fatal(err)
	}

	timeoutErr := errors.New("timeout")
	connRefusedErr := errors.New("connection refused")
	otherErr := errors.New("other error")
	differentErr := errors.New("different error")

	tests := []struct {
		name         string
		qcErr        gorums.QuorumCallError
		errorTypes   []error
		wantExcluded []uint32
	}{
		{
			name:         "ExcludeAllFailedNodes",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr}),
			errorTypes:   nil,
			wantExcluded: []uint32{1, 2},
		},
		{
			name:         "ExcludeNodesWithSpecificError",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr, 3: otherErr}),
			errorTypes:   []error{timeoutErr},
			wantExcluded: []uint32{1},
		},
		{
			name:         "ExcludeNodesWithMultipleErrorTypes",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr, 3: otherErr}),
			errorTypes:   []error{timeoutErr, connRefusedErr},
			wantExcluded: []uint32{1, 2},
		},
		{
			name:         "NoMatchingErrors",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr}),
			errorTypes:   []error{differentErr},
			wantExcluded: []uint32{},
		},
		{
			name:         "EmptyErrors",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{}),
			errorTypes:   nil,
			wantExcluded: []uint32{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCfg := cfg.WithoutErrors(tt.qcErr, tt.errorTypes...)
			// Check that excluded nodes are not in the new configuration
			for _, excludedID := range tt.wantExcluded {
				if newCfg.Contains(excludedID) {
					t.Errorf("newCfg.Contains(%d) = true, want false", excludedID)
				}
			}
			// Check that all other nodes are still in the configuration
			wantSize := cfg.Size() - len(tt.wantExcluded)
			if newCfg.Size() != wantSize {
				t.Errorf("newCfg.Size() = %d, want %d", newCfg.Size(), wantSize)
			}
		})
	}
}
