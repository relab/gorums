package gorums

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// makeTreeNode creates a minimal node for tree layout testing.
func makeTreeNode(id uint32) *Node {
	return &Node{id: id}
}

// makeTreeConfig builds a Configuration with sequential 1-based node IDs,
// matching the rest of the codebase where node ID 0 is reserved.
func makeTreeConfig(n int) Configuration {
	cfg := make(Configuration, n)
	for i := range n {
		cfg[i] = makeTreeNode(uint32(i + 1))
	}
	return cfg
}

func TestAsTree_Errors(t *testing.T) {
	cfg := makeTreeConfig(7)
	tests := []struct {
		name    string
		cfg     Configuration
		opts    TreeOptions
		wantErr string
	}{
		{
			name:    "BranchingFactorZero",
			cfg:     cfg,
			opts:    TreeOptions{BranchingFactor: 0, Depth: 2},
			wantErr: "BranchingFactor must be >= 2",
		},
		{
			name:    "BranchingFactorOne",
			cfg:     cfg,
			opts:    TreeOptions{BranchingFactor: 1, Depth: 2},
			wantErr: "BranchingFactor must be >= 2",
		},
		{
			name:    "DepthZero",
			cfg:     cfg,
			opts:    TreeOptions{BranchingFactor: 2, Depth: 0},
			wantErr: "Depth must be >= 1",
		},
		{
			name:    "EmptyConfig",
			cfg:     Configuration{},
			opts:    TreeOptions{BranchingFactor: 2, Depth: 1},
			wantErr: "empty configuration",
		},
		{
			name:    "NilConfig",
			cfg:     nil,
			opts:    TreeOptions{BranchingFactor: 2, Depth: 1},
			wantErr: "empty configuration",
		},
		{
			name:    "ExcessNodes",
			cfg:     makeTreeConfig(15), // capacity for bf=3, depth=2 is 13
			opts:    TreeOptions{BranchingFactor: 3, Depth: 2},
			wantErr: "exceeds tree capacity",
		},
		{
			name:    "OverflowCapacity",
			cfg:     makeTreeConfig(1),
			opts:    TreeOptions{BranchingFactor: 2, Depth: 63},
			wantErr: "exceeds representable range",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.cfg.AsTree(tt.opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestTreeLevelStart verifies the level-start index arithmetic.
func TestTreeLevelStart(t *testing.T) {
	tests := []struct {
		bf    int
		level int
		want  int
	}{
		// bf=2: 0, 1, 3, 7, 15
		{bf: 2, level: 0, want: 0},
		{bf: 2, level: 1, want: 1},
		{bf: 2, level: 2, want: 3},
		{bf: 2, level: 3, want: 7},
		{bf: 2, level: 4, want: 15},
		// bf=3: 0, 1, 4, 13
		{bf: 3, level: 0, want: 0},
		{bf: 3, level: 1, want: 1},
		{bf: 3, level: 2, want: 4},
		{bf: 3, level: 3, want: 13},
		// bf=4: 0, 1, 5, 21
		{bf: 4, level: 0, want: 0},
		{bf: 4, level: 1, want: 1},
		{bf: 4, level: 2, want: 5},
		{bf: 4, level: 3, want: 21},
	}
	for _, tt := range tests {
		got := treeLevelStart(tt.level, tt.bf)
		if got != tt.want {
			t.Errorf("treeLevelStart(%d, bf=%d) = %d, want %d", tt.level, tt.bf, got, tt.want)
		}
	}
}

// TestTreeParentOf verifies ParentOf on a perfect bf=3 depth=2 tree.
func TestTreeParentOf(t *testing.T) {
	tree := mustNewTree(t, 13, TreeOptions{BranchingFactor: 3, Depth: 2})
	tests := []struct {
		id         uint32
		wantParent uint32 // ignored when wantNil is true
		wantNil    bool
	}{
		{id: 1, wantNil: true}, // root has no parent
		{id: 2, wantParent: 1}, // children of root
		{id: 3, wantParent: 1},
		{id: 4, wantParent: 1},
		{id: 5, wantParent: 2}, // children of node 2
		{id: 6, wantParent: 2},
		{id: 7, wantParent: 2},
		{id: 8, wantParent: 3}, // children of node 3
		{id: 9, wantParent: 3},
		{id: 10, wantParent: 3},
		{id: 11, wantParent: 4}, // children of node 4
		{id: 12, wantParent: 4},
		{id: 13, wantParent: 4},
		{id: 14, wantNil: true}, // not in tree
	}
	for _, tt := range tests {
		got := tree.ParentOf(tt.id)
		if tt.wantNil {
			if got != nil {
				t.Errorf("ParentOf(%d) = node %d, want nil", tt.id, got.ID())
			}
		} else {
			if got == nil {
				t.Errorf("ParentOf(%d) = nil, want node %d", tt.id, tt.wantParent)
			} else if got.ID() != tt.wantParent {
				t.Errorf("ParentOf(%d) = node %d, want node %d", tt.id, got.ID(), tt.wantParent)
			}
		}
	}
}

// TestTreeChildrenOf verifies ChildrenOf on a perfect bf=3 depth=2 tree.
func TestTreeChildrenOf(t *testing.T) {
	tree := mustNewTree(t, 13, TreeOptions{BranchingFactor: 3, Depth: 2})
	tests := []struct {
		id      uint32
		wantIDs []uint32
	}{
		{id: 1, wantIDs: []uint32{2, 3, 4}}, // root
		{id: 2, wantIDs: []uint32{5, 6, 7}},
		{id: 3, wantIDs: []uint32{8, 9, 10}},
		{id: 4, wantIDs: []uint32{11, 12, 13}},
		{id: 5, wantIDs: nil}, // leaves
		{id: 6, wantIDs: nil},
		{id: 7, wantIDs: nil},
		{id: 8, wantIDs: nil},
		{id: 9, wantIDs: nil},
		{id: 10, wantIDs: nil},
		{id: 11, wantIDs: nil},
		{id: 12, wantIDs: nil},
		{id: 13, wantIDs: nil},
		{id: 14, wantIDs: nil}, // not in tree
	}
	for _, tt := range tests {
		got := tree.ChildrenOf(tt.id)
		if !slices.Equal(got.NodeIDs(), tt.wantIDs) {
			t.Errorf("ChildrenOf(%d) = %v, want %v", tt.id, got.NodeIDs(), tt.wantIDs)
		}
	}
}

// TestTreeSubtree verifies Subtree on a perfect bf=3 depth=2 tree.
func TestTreeSubtree(t *testing.T) {
	tree := mustNewTree(t, 13, TreeOptions{BranchingFactor: 3, Depth: 2})
	tests := []struct {
		id      uint32
		wantIDs []uint32
	}{
		{id: 1, wantIDs: []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}}, // full tree
		{id: 2, wantIDs: []uint32{2, 5, 6, 7}},
		{id: 3, wantIDs: []uint32{3, 8, 9, 10}},
		{id: 4, wantIDs: []uint32{4, 11, 12, 13}},
		{id: 5, wantIDs: []uint32{5}}, // leaves: just self
		{id: 6, wantIDs: []uint32{6}},
		{id: 7, wantIDs: []uint32{7}},
		{id: 8, wantIDs: []uint32{8}},
		{id: 9, wantIDs: []uint32{9}},
		{id: 10, wantIDs: []uint32{10}},
		{id: 11, wantIDs: []uint32{11}},
		{id: 12, wantIDs: []uint32{12}},
		{id: 13, wantIDs: []uint32{13}},
		{id: 14, wantIDs: nil}, // not in tree
	}
	for _, tt := range tests {
		got := tree.Subtree(tt.id)
		if !slices.Equal(got.NodeIDs(), tt.wantIDs) {
			t.Errorf("Subtree(%d) = %v, want %v", tt.id, got.NodeIDs(), tt.wantIDs)
		}
	}
}

// TestTreePartialLastLevel verifies layout when the configuration is smaller
// than a perfect tree (bf=3, depth=2, only 10 of 13 nodes present).
//
//	     1 (root)
//	   /    |    \
//	  2     3     4
//	 /|\   /|\
//	5 6 7 8 9 10
func TestTreePartialLastLevel(t *testing.T) {
	tree := mustNewTree(t, 10, TreeOptions{BranchingFactor: 3, Depth: 2})

	// Node 2 (level 1, idx 0): children at positions 4,5,6 — all present.
	if got := tree.ChildrenOf(2); !slices.Equal(got.NodeIDs(), []uint32{5, 6, 7}) {
		t.Errorf("ChildrenOf(2) = %v, want [5 6 7]", got.NodeIDs())
	}
	// Node 3 (level 1, idx 1): children at positions 7,8,9 — all present.
	if got := tree.ChildrenOf(3); !slices.Equal(got.NodeIDs(), []uint32{8, 9, 10}) {
		t.Errorf("ChildrenOf(3) = %v, want [8 9 10]", got.NodeIDs())
	}
	// Node 4 (level 1, idx 2): children would be at 10,11,12 — none present.
	if got := tree.ChildrenOf(4); got != nil {
		t.Errorf("ChildrenOf(4) = %v, want nil", got.NodeIDs())
	}
	// Subtree(4) = just node 4.
	if got := tree.Subtree(4); !slices.Equal(got.NodeIDs(), []uint32{4}) {
		t.Errorf("Subtree(4) = %v, want [4]", got.NodeIDs())
	}
	// Subtree of root spans every present node.
	if got := tree.Subtree(1); !slices.Equal(got.NodeIDs(), []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) {
		t.Errorf("Subtree(1) = %v, want [1..10]", got.NodeIDs())
	}
}

// TestTreePartialLastLevel_OneChild verifies the case where the last internal
// node has fewer children than bf (bf=3, depth=2, 12 nodes: node 4 has 2
// children instead of 3).
//
//	     1 (root)
//	   /    |      \
//	  2     3       4
//	 /|\   /|\    / |
//	5 6 7 8 9 10 11 12
func TestTreePartialLastLevel_OneChild(t *testing.T) {
	tree := mustNewTree(t, 12, TreeOptions{BranchingFactor: 3, Depth: 2})
	// Node 4 (level 1, idx 2): children at 11, 12 — position 12 (slot for ID 13) absent.
	if got := tree.ChildrenOf(4); !slices.Equal(got.NodeIDs(), []uint32{11, 12}) {
		t.Errorf("ChildrenOf(4) = %v, want [11 12]", got.NodeIDs())
	}
}

// TestTreeExcessNodes verifies that a configuration larger than the tree
// capacity is rejected (bf=3, depth=2, capacity=13; give 15 nodes).
func TestTreeExcessNodes(t *testing.T) {
	_, err := makeTreeConfig(15).AsTree(TreeOptions{BranchingFactor: 3, Depth: 2})
	if err == nil {
		t.Fatal("expected error for configuration exceeding tree capacity, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds tree capacity") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestTreeBF2Depth3 exercises a perfect binary tree (bf=2, depth=3, 15 nodes).
//
//	        1 (root)
//	      /         \
//	     2           3
//	   /   \       /   \
//	  4     5     6     7
//	 / \   / \   / \   / \
//	8   9 10 11 12 13 14 15
func TestTreeBF2Depth3(t *testing.T) {
	tree := mustNewTree(t, 15, TreeOptions{BranchingFactor: 2, Depth: 3})

	parentTests := []struct {
		id     uint32
		wantID uint32
		nilOK  bool
	}{
		{1, 0, true},
		{2, 1, false},
		{3, 1, false},
		{4, 2, false},
		{5, 2, false},
		{6, 3, false},
		{7, 3, false},
		{8, 4, false},
		{9, 4, false},
		{15, 7, false},
	}
	for _, tt := range parentTests {
		p := tree.ParentOf(tt.id)
		if tt.nilOK {
			if p != nil {
				t.Errorf("ParentOf(%d) = %d, want nil", tt.id, p.ID())
			}
		} else if p == nil || p.ID() != tt.wantID {
			got := uint32(0)
			if p != nil {
				got = p.ID()
			}
			t.Errorf("ParentOf(%d) = %d, want %d", tt.id, got, tt.wantID)
		}
	}

	// Subtree of node 2: {2, 4, 5, 8, 9, 10, 11}
	if got := tree.Subtree(2); !slices.Equal(got.NodeIDs(), []uint32{2, 4, 5, 8, 9, 10, 11}) {
		t.Errorf("Subtree(2) = %v, want [2 4 5 8 9 10 11]", got.NodeIDs())
	}
}

// TestServerCtxTree verifies the ServerCtx tree accessors against the
// bf=3 depth=2 tree used throughout this file.
//
//	     1 (root)
//	   /    |     \
//	  2     3      4
//	 /|\   /|\   / | \
//	5 6 7 8 9 10 11 12 13
func TestServerCtxTree(t *testing.T) {
	tree := mustNewTree(t, 13, TreeOptions{BranchingFactor: 3, Depth: 2})

	// serverCtxFor builds a minimal ServerCtx whose srv.myID is set to id.
	serverCtxFor := func(id uint32) ServerCtx {
		s := &Server{
			inboundManager: &inboundManager{myID: id},
			tree:           tree,
		}
		return ServerCtx{Context: context.Background(), srv: s}
	}

	t.Run("Root", func(t *testing.T) {
		ctx := serverCtxFor(1)
		if got := ctx.TreeChildren(); !slices.Equal(got.NodeIDs(), []uint32{2, 3, 4}) {
			t.Errorf("TreeChildren = %v, want [2 3 4]", got.NodeIDs())
		}
		if p := ctx.TreeParent(); p != nil {
			t.Errorf("TreeParent = node %d, want nil", p.ID())
		}
	})

	t.Run("InternalNode", func(t *testing.T) {
		ctx := serverCtxFor(3) // level 1, idx 1; children=[8,9,10]; parent=1
		if got := ctx.TreeChildren(); !slices.Equal(got.NodeIDs(), []uint32{8, 9, 10}) {
			t.Errorf("TreeChildren = %v, want [8 9 10]", got.NodeIDs())
		}
		if p := ctx.TreeParent(); p == nil || p.ID() != 1 {
			t.Errorf("TreeParent = %v, want node 1", p)
		}
	})

	t.Run("Leaf", func(t *testing.T) {
		ctx := serverCtxFor(8) // level 2, idx 3; no children; parent=3
		if got := ctx.TreeChildren(); got != nil {
			t.Errorf("TreeChildren = %v, want nil", got.NodeIDs())
		}
		if p := ctx.TreeParent(); p == nil || p.ID() != 3 {
			t.Errorf("TreeParent = %v, want node 3", p)
		}
	})

	t.Run("NodeNotInTree", func(t *testing.T) {
		ctx := serverCtxFor(99) // ID not present in the 13-node tree
		if got := ctx.TreeChildren(); got != nil {
			t.Errorf("TreeChildren = %v, want nil", got.NodeIDs())
		}
		if p := ctx.TreeParent(); p != nil {
			t.Errorf("TreeParent = node %d, want nil", p.ID())
		}
	})

	t.Run("NoTreeRegistered", func(t *testing.T) {
		s := &Server{inboundManager: &inboundManager{myID: 1}}
		ctx := ServerCtx{Context: context.Background(), srv: s}
		if got := ctx.TreeChildren(); got != nil {
			t.Errorf("TreeChildren = %v, want nil (no tree)", got.NodeIDs())
		}
		if p := ctx.TreeParent(); p != nil {
			t.Errorf("TreeParent = node %d, want nil (no tree)", p.ID())
		}
	})
}

// TestTreeContext verifies that Context returns a ConfigContext addressing
// the root's direct children on a perfect bf=3 depth=2 tree.
func TestTreeContext(t *testing.T) {
	tree := mustNewTree(t, 13, TreeOptions{BranchingFactor: 3, Depth: 2})
	ctx := tree.Context(context.Background())
	if ctx == nil {
		t.Fatal("Context returned nil")
	}
	want := []uint32{2, 3, 4}
	if got := ctx.Configuration().NodeIDs(); !slices.Equal(got, want) {
		t.Errorf("Context.Configuration().NodeIDs() = %v, want %v", got, want)
	}
}

// TestTreeContext_PanicsOnRootWithNoChildren verifies that Context panics
// when the configuration contains only the root (no children present).
func TestTreeContext_PanicsOnRootWithNoChildren(t *testing.T) {
	tree := mustNewTree(t, 1, TreeOptions{BranchingFactor: 2, Depth: 1})
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Context did not panic on a tree with no children")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "no children") {
			t.Errorf("panic = %v, want string containing %q", r, "no children")
		}
	}()
	_ = tree.Context(context.Background())
}

// mustNewTree creates a TreeConfiguration for testing, failing the test on error.
func mustNewTree(t *testing.T, n int, opts TreeOptions) *TreeConfiguration {
	t.Helper()
	tree, err := makeTreeConfig(n).AsTree(opts)
	if err != nil {
		t.Fatalf("AsTree: %v", err)
	}
	return tree
}
