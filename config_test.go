package gorums_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
)

var (
	nodeList = []string{"127.0.0.1:9081", "127.0.0.1:9082", "127.0.0.1:9083"}
	nodeMap  = map[uint32]mock.NodeAddr{
		1: "127.0.0.1:9081",
		2: "127.0.0.1:9082",
		3: "127.0.0.1:9083",
		4: "127.0.0.1:9084",
	}
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name     string
		nodes    gorums.NodeSource
		wantSize int
		wantErr  string
	}{
		{
			name:     "WithNodeList/Success",
			nodes:    gorums.WithNodeList(nodeList),
			wantSize: len(nodeList),
		},
		{
			name:     "WithNodes/Success",
			nodes:    gorums.WithNodes(nodeMap),
			wantSize: len(nodeMap),
		},
		{
			name:    "WithNodeList/Reject/EmptyNodeList",
			nodes:   gorums.WithNodeList([]string{}),
			wantErr: "gorums: missing required node addresses",
		},
		{
			name:    "WithNodes/Reject/EmptyNodeMap",
			nodes:   gorums.WithNodes(map[uint32]mock.NodeAddr{}),
			wantErr: "gorums: missing required node map",
		},
		{
			name: "WithNodes/Reject/ZeroID",
			nodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				0: "127.0.0.1:9080", // ID 0 should be rejected
				1: "127.0.0.1:9081",
			}),
			wantErr: "gorums: node 0 is reserved",
		},
		{
			name: "WithNodes/Reject/DuplicateAddress",
			nodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				1: "127.0.0.1:9081",
				2: "127.0.0.1:9081", // Duplicate address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodeList/Reject/DuplicateAddress",
			nodes: gorums.WithNodeList([]string{
				"127.0.0.1:9081",
				"127.0.0.1:9081", // Duplicate address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodes/Reject/NormalizedDuplicateAddress",
			nodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				1: "localhost:9081",
				2: "127.0.0.1:9081", // Same resolved address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name: "WithNodeList/Reject/NormalizedDuplicateAddress",
			nodes: gorums.WithNodeList([]string{
				"localhost:9081",
				"127.0.0.1:9081", // Same resolved address
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := gorums.NewConfig(tt.nodes, gorumstest.InsecureDialOptions(t))
			if err == nil {
				t.Cleanup(gorumstest.Closer(t, cfg))
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

// TestNewConfigWithBackChannel verifies that NewConfig accepts a handler-only
// back-channel server and rejects a nil server or a server configured with
// WithPeers.
func TestNewConfigWithBackChannel(t *testing.T) {
	tests := []struct {
		name    string
		srv     func(t *testing.T) *gorums.Server
		wantErr string
	}{
		{
			name: "Accept/HandlerOnlyServer",
			srv:  func(*testing.T) *gorums.Server { return gorums.NewServer() },
		},
		{
			name:    "Reject/NilServer",
			srv:     func(*testing.T) *gorums.Server { return nil },
			wantErr: "gorums: WithBackChannel requires a non-nil server",
		},
		{
			name: "Reject/PeerServer",
			srv: func(t *testing.T) *gorums.Server {
				return gorums.NewServer(gorums.WithPeers(
					1, gorums.WithNodeList(nodeList), gorumstest.InsecureDialOptions(t),
				))
			},
			wantErr: "gorums: WithBackChannel server must not be configured with WithPeers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.srv(t)
			if srv != nil {
				t.Cleanup(srv.Stop)
			}
			cfg, err := gorums.NewConfig(
				gorums.WithNodeList(nodeList),
				gorums.WithBackChannel(srv),
				gorumstest.InsecureDialOptions(t),
			)
			if err == nil {
				t.Cleanup(gorumstest.Closer(t, cfg))
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
			if cfg.Size() != len(nodeList) {
				t.Errorf("cfg.Size() = %d, want %d", cfg.Size(), len(nodeList))
			}
		})
	}
}

func TestEmptyConfiguration(t *testing.T) {
	var empty gorums.Config
	populated := gorumstest.UnreachableConfig(t, nodeList...)

	t.Run("ContextPanics", func(t *testing.T) {
		assertPanic(t, "gorums: Context called on an empty configuration", func() {
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
		var otherEmpty gorums.Config
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
		var otherEmpty gorums.Config
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
		qcErr := gorumstest.QuorumCallError(map[uint32]error{1: errors.New("boom")})
		if got := empty.WithoutErrors(qcErr); got != nil {
			t.Fatalf("empty.WithoutErrors(...) = %v, want nil", got)
		}
	})
}

// assertPanic fails the test unless fn panics with a message that starts
// with wantPrefix.
func assertPanic(t *testing.T, wantPrefix string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic %q, got no panic", wantPrefix)
		}
		if got := fmt.Sprint(r); !strings.HasPrefix(got, wantPrefix) {
			t.Fatalf("panic = %q, want prefix %q", got, wantPrefix)
		}
	}()
	fn()
}

func TestConfigExtend(t *testing.T) {
	initialNodes := gorums.WithNodeList(nodeList[:2]) // {1,2}

	tests := []struct {
		name         string
		initialNodes gorums.NodeSource
		extendNodes  gorums.NodeSource
		wantSize     int
		wantErr      string
	}{
		{
			name:         "WithNil/Success",
			initialNodes: initialNodes,
			extendNodes:  nil,
			wantSize:     2,
		},
		{
			name:         "WithNodeList/Success",
			initialNodes: initialNodes,
			extendNodes:  gorums.WithNodeList(nodeList[2:]),
			wantSize:     len(nodeList),
		},
		{
			name:         "WithNodes/Success",
			initialNodes: initialNodes,
			extendNodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				10: "127.0.0.1:9090",
				11: "127.0.0.1:9091",
			}),
			wantSize: 4, // 2 initial + 2 new
		},
		{
			name:         "WithNodes/Reject/ZeroID",
			initialNodes: initialNodes,
			extendNodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				0: "127.0.0.1:9090", // ID 0 should be rejected
			}),
			wantErr: "gorums: node 0 is reserved",
		},
		{
			name:         "WithNodes/Reject/IDConflict",
			initialNodes: initialNodes,
			extendNodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				2: "127.0.0.1:9090", // ID 2 already exists, rejected
			}),
			wantErr: `gorums: node 2 already in use by "127.0.0.1:9082"`,
		},
		{
			name:         "WithNodes/Reject/AddressConflict",
			initialNodes: initialNodes,
			extendNodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				3: "127.0.0.1:9081", // Same address as ID 1
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
		{
			name:         "WithNodes/Reject/NormalizedAddressConflict",
			initialNodes: initialNodes,
			extendNodes: gorums.WithNodes(map[uint32]mock.NodeAddr{
				3: "localhost:9081", // Resolves to same as existing node 1
			}),
			wantErr: `gorums: address "127.0.0.1:9081" already in use by node 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := gorums.NewConfig(tt.initialNodes, gorumstest.InsecureDialOptions(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(gorumstest.Closer(t, c))

			c2, err := c.Extend(tt.extendNodes)
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

func TestConfigExtendConcurrent(t *testing.T) {
	addrs := gorumstest.Servers(t, 6, func(_ int) gorumstest.ServerIface { return gorums.NewServer() })

	// Create base configuration so that concurrent Extend operations share the same node registry.
	cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs[0:1]), gorumstest.DialOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorumstest.Closer(t, cfg))

	// Create multiple node maps to extend with, each containing a unique new node.
	// These maps will be used concurrently to verify that Extend can safely mutate
	// the shared node registry under concurrent use (race-free configuration creation).
	nodeMaps := []map[uint32]mock.NodeAddr{
		{2: mock.NodeAddr(addrs[1])},
		{3: mock.NodeAddr(addrs[2])},
		{4: mock.NodeAddr(addrs[3])},
		{5: mock.NodeAddr(addrs[4])},
		{6: mock.NodeAddr(addrs[5])},
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

// TestConfigClose verifies that Close leaves no live node behind when Extend
// runs after or concurrently with Close.
func TestConfigClose(t *testing.T) {
	t.Run("ExtendAfterClose", func(t *testing.T) {
		addrs := gorumstest.Servers(t, 2, nil)
		cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs[:1]), gorumstest.DialOptions(t))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(gorumstest.Closer(t, cfg))
		if err := cfg.Close(); err != nil {
			t.Fatalf("cfg.Close() = %v, want nil", err)
		}

		ext, err := cfg.Extend(gorums.WithNodeList(addrs[1:]))
		if err == nil {
			t.Cleanup(gorumstest.Closer(t, ext))
			t.Fatalf("cfg.Extend() after Close = %v, nil; want error", ext.NodeIDs())
		}
		if ext != nil {
			t.Errorf("cfg.Extend() after Close = %v, want nil configuration", ext.NodeIDs())
		}
	})

	t.Run("ConcurrentExtend", func(t *testing.T) {
		addrs := gorumstest.Servers(t, 2, nil)
		for range 10 {
			cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs[:1]), gorumstest.DialOptions(t))
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			wg.Go(func() {
				if err := cfg.Close(); err != nil {
					t.Errorf("cfg.Close() = %v, want nil", err)
				}
			})
			wg.Go(func() {
				// Extend either joins the pool before Close takes its snapshot
				// and is closed with it, or it fails. Neither outcome may leave
				// a live node behind; goleak checks that at cleanup.
				_, _ = cfg.Extend(gorums.WithNodeList(addrs[1:]))
			})
			wg.Wait()
		}
	})
}

func TestConfigAdd(t *testing.T) {
	c1 := gorumstest.UnreachableConfig(t, nodeList...) // c1 = {1, 2, 3}
	if c1.Size() != len(nodeList) {
		t.Errorf("c1.Size() = %d, want %d", c1.Size(), len(nodeList))
	}

	// Add existing c1 IDs (should return same configuration)
	nodeIDs := c1.NodeIDs()
	c2 := c1.Add(nodeIDs...) // c2 = {1, 2, 3}
	if c2.Size() != len(nodeList) {
		t.Errorf("c2.Size() = %d, want %d", c2.Size(), len(nodeList))
	}
	if !c1.Equal(c2) {
		t.Errorf("c1.Equal(c2) = false, want true")
	}

	// Add non-existent ID (should be ignored)
	c3 := c1.Add(999) // c3 = {1, 2, 3}
	if c3.Size() != len(nodeList) {
		t.Errorf("c3.Size() = %d, want %d", c3.Size(), len(nodeList))
	}
}

func TestConfigUnion(t *testing.T) {
	c1 := gorumstest.UnreachableConfig(t, nodeList...) // c1 = {1, 2, 3}

	// Add newNodes to c1 using Extend (gets IDs 4, 5)
	newNodes := []string{"127.0.0.1:9084", "127.0.0.1:9085"}
	c3, err := c1.Extend(gorums.WithNodeList(newNodes)) // c3 = {1, 2, 3, 4, 5}
	if err != nil {
		t.Fatal(err)
	}
	if c3.Size() != len(nodeList)+len(newNodes) {
		t.Errorf("c3.Size() = %d, want %d", c3.Size(), len(nodeList)+len(newNodes))
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

func TestConfigRemove(t *testing.T) {
	c1 := gorumstest.UnreachableConfig(t, nodeList...) // c1 = {1, 2, 3}

	// Remove one node using Remove
	c2 := c1.Remove(c1[0].ID()) // c2 = {2, 3}
	if c2.Size() != c1.Size()-1 {
		t.Errorf("c2.Size() = %d, want %d", c2.Size(), c1.Size()-1)
	}
}

func TestConfigDifference(t *testing.T) {
	c1 := gorumstest.UnreachableConfig(t, nodeList...) // c1 = {1, 2, 3}

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

func TestConfigAddDuplicateIDs(t *testing.T) {
	c2 := gorumstest.UnreachableConfig(t, nodeList...) // c2 = {1, 2, 3}
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

func TestConfigUnionDuplicateNodes(t *testing.T) {
	c1 := gorumstest.UnreachableConfig(t, nodeList...) // c1 = {1, 2, 3}

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

func TestConfigImmutability(t *testing.T) {
	c1 := gorumstest.UnreachableConfig(t, nodeList...) // c1 = {1, 2, 3}

	// Test Union with empty returns a clone, not the original
	var emptyConfig gorums.Config
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

func TestConfigWithoutErrors(t *testing.T) {
	cfg, err := gorums.NewConfig(gorums.WithNodes(nodeMap), gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorumstest.Closer(t, cfg))

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
			qcErr:        gorumstest.QuorumCallError(map[uint32]error{1: timeoutErr, 2: connRefusedErr}),
			errorTypes:   nil,
			wantExcluded: []uint32{1, 2},
		},
		{
			name:         "ExcludeNodesWithSpecificError",
			qcErr:        gorumstest.QuorumCallError(map[uint32]error{1: timeoutErr, 2: connRefusedErr, 3: otherErr}),
			errorTypes:   []error{timeoutErr},
			wantExcluded: []uint32{1},
		},
		{
			name:         "ExcludeNodesWithMultipleErrorTypes",
			qcErr:        gorumstest.QuorumCallError(map[uint32]error{1: timeoutErr, 2: connRefusedErr, 3: otherErr}),
			errorTypes:   []error{timeoutErr, connRefusedErr},
			wantExcluded: []uint32{1, 2},
		},
		{
			name:         "NoMatchingErrors",
			qcErr:        gorumstest.QuorumCallError(map[uint32]error{1: timeoutErr, 2: connRefusedErr}),
			errorTypes:   []error{differentErr},
			wantExcluded: []uint32{},
		},
		{
			name:         "EmptyErrors",
			qcErr:        gorumstest.QuorumCallError(map[uint32]error{}),
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
