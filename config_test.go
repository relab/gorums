package gorums_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/relab/gorums"
)

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

func TestNewConfig(t *testing.T) {
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
			wantErr: "gorums: missing required node addresses",
		},
		{
			name:    "WithNodes/Reject/EmptyNodeMap",
			opt:     gorums.WithNodes(map[uint32]testNode{}),
			wantErr: "gorums: missing required node map",
		},
		{
			name: "WithNodes/Reject/ZeroID",
			opt: gorums.WithNodes(map[uint32]testNode{
				0: {addr: "127.0.0.1:9080"}, // ID 0 should be rejected
				1: {addr: "127.0.0.1:9081"},
			}),
			wantErr: "gorums: node 0 is reserved",
		},
		{
			name: "WithNodes/Reject/DuplicateAddress",
			opt: gorums.WithNodes(map[uint32]testNode{
				1: {addr: "127.0.0.1:9081"},
				2: {addr: "127.0.0.1:9081"}, // Duplicate address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodeList/Reject/DuplicateAddress",
			opt: gorums.WithNodeList([]string{
				"127.0.0.1:9081",
				"127.0.0.1:9081", // Duplicate address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodes/Reject/NormalizedDuplicateAddress",
			opt: gorums.WithNodes(map[uint32]testNode{
				1: {addr: "localhost:9081"},
				2: {addr: "127.0.0.1:9081"}, // Same resolved address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodeList/Reject/NormalizedDuplicateAddress",
			opt: gorums.WithNodeList([]string{
				"localhost:9081",
				"127.0.0.1:9081", // Same resolved address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := gorums.NewConfig(tt.opt, gorums.InsecureDialOptions(t))
			if err == nil {
				t.Cleanup(gorums.Closer(t, cfg))
			}
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
		})
	}
}

func TestEmptyConfiguration(t *testing.T) {
	var empty gorums.Configuration

	populated, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, populated))

	t.Run("ContextPanics", func(t *testing.T) {
		assertPanicMessage(t, "gorums: Context called on an empty configuration", func() {
			_ = empty.Context(t.Context())
		})
	})

	t.Run("ExtendReturnsError", func(t *testing.T) {
		got, err := empty.Extend(nil)
		if err == nil {
			t.Fatal("empty.Extend(nil) error = nil, want non-nil")
		}
		wantErr := "gorums: cannot extend empty configuration"
		if err.Error() != wantErr {
			t.Fatalf("empty.Extend(nil) error = %q, want %q", err.Error(), wantErr)
		}
		if got != nil {
			t.Fatalf("empty.Extend(nil) cfg = %v, want nil", got)
		}
	})

	t.Run("NodeIDsEmpty", func(t *testing.T) {
		got := empty.NodeIDs()
		if len(got) != 0 {
			t.Fatalf("len(empty.NodeIDs()) = %d, want 0", len(got))
		}
		if got == nil {
			t.Fatal("empty.NodeIDs() = nil, want empty slice")
		}
	})

	t.Run("NodesNil", func(t *testing.T) {
		if got := empty.Nodes(); got != nil {
			t.Fatalf("empty.Nodes() = %v, want nil", got)
		}
	})

	t.Run("SizeZero", func(t *testing.T) {
		if got := empty.Size(); got != 0 {
			t.Fatalf("empty.Size() = %d, want 0", got)
		}
	})

	t.Run("Equal", func(t *testing.T) {
		var otherEmpty gorums.Configuration
		if !empty.Equal(otherEmpty) {
			t.Fatal("empty.Equal(otherEmpty) = false, want true")
		}
		if empty.Equal(populated) {
			t.Fatal("empty.Equal(populated) = true, want false")
		}
	})

	t.Run("CloseNil", func(t *testing.T) {
		if err := empty.Close(); err != nil {
			t.Fatalf("empty.Close() error = %v, want nil", err)
		}
	})

	t.Run("ContainsFalse", func(t *testing.T) {
		if empty.Contains(1) {
			t.Fatal("empty.Contains(1) = true, want false")
		}
	})

	t.Run("AddNil", func(t *testing.T) {
		if got := empty.Add(1, 2, 3); got != nil {
			t.Fatalf("empty.Add(...) = %v, want nil", got)
		}
	})

	t.Run("UnionWithEmptyNil", func(t *testing.T) {
		var otherEmpty gorums.Configuration
		if got := empty.Union(otherEmpty); got != nil {
			t.Fatalf("empty.Union(otherEmpty) = %v, want nil", got)
		}
	})

	t.Run("UnionWithNonEmptyClonesOther", func(t *testing.T) {
		got := empty.Union(populated)
		if !got.Equal(populated) {
			t.Fatal("empty.Union(populated) != populated")
		}
		gotNodes := got.Nodes()
		populatedNodes := populated.Nodes()
		if len(gotNodes) == 0 {
			t.Fatal("empty.Union(populated) returned empty configuration")
		}
		if &gotNodes[0] == &populatedNodes[0] {
			t.Fatal("empty.Union(populated) shares backing array with populated")
		}
	})

	t.Run("RemoveNil", func(t *testing.T) {
		if got := empty.Remove(1, 2, 3); got != nil {
			t.Fatalf("empty.Remove(...) = %v, want nil", got)
		}
	})

	t.Run("DifferenceNil", func(t *testing.T) {
		if got := empty.Difference(populated); got != nil {
			t.Fatalf("empty.Difference(populated) = %v, want nil", got)
		}
	})

	t.Run("WithoutErrorsNil", func(t *testing.T) {
		qcErr := gorums.TestQuorumCallError(t, map[uint32]error{1: errors.New("boom")})
		if got := empty.WithoutErrors(qcErr); got != nil {
			t.Fatalf("empty.WithoutErrors(...) = %v, want nil", got)
		}
	})
}

func assertPanicMessage(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic %q, got no panic", want)
		}
		if got := fmt.Sprint(r); got != want {
			t.Fatalf("panic = %q, want %q", got, want)
		}
	}()
	fn()
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
			wantErr: "gorums: node 0 is reserved",
		},
		{
			name:         "WithNodes/Reject/IDConflict",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				2: {addr: "127.0.0.1:9090"}, // ID 2 already exists, rejected
			}),
			wantErr: `gorums: node 2 already in use by "127.0.0.1:9082"`,
		},
		{
			name:         "WithNodes/Reject/AddressConflict",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				3: {addr: "127.0.0.1:9081"}, // Same address as ID 1
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name:         "WithNodes/Reject/NormalizedAddressConflict",
			initialNodes: nodes12,
			extendOpt: gorums.WithNodes(map[uint32]testNode{
				3: {addr: "localhost:9081"}, // Resolves to same as existing node 1
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := gorums.NewConfig(gorums.WithNodeList(tt.initialNodes), gorums.InsecureDialOptions(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(gorums.Closer(t, c))

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
		})
	}
}

func TestConfigurationExtendConcurrent(t *testing.T) {
	addrs := gorums.TestServers(t, 6, func(_ int) gorums.ServerIface { return gorums.NewServer() })

	// Create base configuration so that concurrent Extend operations share the same node registry.
	cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs[0:1]), gorums.TestDialOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, cfg))

	// Create multiple node maps to extend with, each containing a unique new node.
	// These maps will be used concurrently to verify that Extend can safely mutate
	// the shared node registry under concurrent use (race-free configuration creation).
	nodeMaps := []map[uint32]testNode{
		{2: {addr: addrs[1]}},
		{3: {addr: addrs[2]}},
		{4: {addr: addrs[3]}},
		{5: {addr: addrs[4]}},
		{6: {addr: addrs[5]}},
	}

	errCh := make(chan error, len(nodeMaps))
	var wg sync.WaitGroup
	for i := range nodeMaps {
		wg.Go(func() {
			// Exercise concurrent configuration creation against the same node registry.
			c, err := cfg.Extend(gorums.WithNodes(nodeMaps[i]))
			if err != nil {
				errCh <- err
				return
			}
			if c.Size() != 2 {
				errCh <- fmt.Errorf("c.Size() = %d, want 2", c.Size())
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestConfigurationAdd(t *testing.T) {
	c1, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, c1))
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
	c1, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, c1))

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
	c1, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, c1))

	// Remove one node using Remove
	c2 := c1.Remove(c1[0].ID()) // c2 = {2, 3}
	if c2.Size() != c1.Size()-1 {
		t.Errorf("c2.Size() = %d, want %d", c2.Size(), c1.Size()-1)
	}
}

func TestConfigurationDifference(t *testing.T) {
	c1, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, c1))

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
	// Create configuration with all three nodes
	c2, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t)) // c2 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, c2))
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
	// Create configuration with nodes 1, 2, 3
	c1, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, c1))

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
	c1, err := gorums.NewConfig(gorums.WithNodeList(nodes), gorums.InsecureDialOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, c1))

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
	cfg, err := gorums.NewConfig(gorums.WithNodes(nodeMap), gorums.InsecureDialOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorums.Closer(t, cfg))

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
