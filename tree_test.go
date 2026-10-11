package gorums

import (
	"slices"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/conn"
)

// makeTreeNode creates a minimal node for tree layout testing.
func makeTreeNode(id ID) *Node {
	return conn.NewNodeForTest(id, nil)
}

// makeTreeConfig builds a Config with sequential 1-based node IDs,
// matching the rest of the codebase where node ID 0 is reserved.
func makeTreeConfig(n int) Config {
	cfg := make(Config, n)
	for i := range n {
		cfg[i] = makeTreeNode(ID(i + 1))
	}
	return cfg
}

func TestNewTree_Errors(t *testing.T) {
	cfg := makeTreeConfig(7)
	tests := []struct {
		name    string
		cfg     Config
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
			cfg:     Config{},
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
			_, err := NewTree(tt.cfg, tt.opts)
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
		id         ID
		wantParent ID // ignored when wantNil is true
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
		id      ID
		wantIDs []ID
	}{
		{id: 1, wantIDs: []ID{2, 3, 4}}, // root
		{id: 2, wantIDs: []ID{5, 6, 7}},
		{id: 3, wantIDs: []ID{8, 9, 10}},
		{id: 4, wantIDs: []ID{11, 12, 13}},
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
		id      ID
		wantIDs []ID
	}{
		{id: 1, wantIDs: []ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}}, // full tree
		{id: 2, wantIDs: []ID{2, 5, 6, 7}},
		{id: 3, wantIDs: []ID{3, 8, 9, 10}},
		{id: 4, wantIDs: []ID{4, 11, 12, 13}},
		{id: 5, wantIDs: []ID{5}}, // leaves: just self
		{id: 6, wantIDs: []ID{6}},
		{id: 7, wantIDs: []ID{7}},
		{id: 8, wantIDs: []ID{8}},
		{id: 9, wantIDs: []ID{9}},
		{id: 10, wantIDs: []ID{10}},
		{id: 11, wantIDs: []ID{11}},
		{id: 12, wantIDs: []ID{12}},
		{id: 13, wantIDs: []ID{13}},
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
	if got := tree.ChildrenOf(2); !slices.Equal(got.NodeIDs(), []ID{5, 6, 7}) {
		t.Errorf("ChildrenOf(2) = %v, want [5 6 7]", got.NodeIDs())
	}
	// Node 3 (level 1, idx 1): children at positions 7,8,9 — all present.
	if got := tree.ChildrenOf(3); !slices.Equal(got.NodeIDs(), []ID{8, 9, 10}) {
		t.Errorf("ChildrenOf(3) = %v, want [8 9 10]", got.NodeIDs())
	}
	// Node 4 (level 1, idx 2): children would be at 10,11,12 — none present.
	if got := tree.ChildrenOf(4); got != nil {
		t.Errorf("ChildrenOf(4) = %v, want nil", got.NodeIDs())
	}
	// Subtree(4) = just node 4.
	if got := tree.Subtree(4); !slices.Equal(got.NodeIDs(), []ID{4}) {
		t.Errorf("Subtree(4) = %v, want [4]", got.NodeIDs())
	}
	// Subtree of root spans every present node.
	if got := tree.Subtree(1); !slices.Equal(got.NodeIDs(), []ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) {
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
	if got := tree.ChildrenOf(4); !slices.Equal(got.NodeIDs(), []ID{11, 12}) {
		t.Errorf("ChildrenOf(4) = %v, want [11 12]", got.NodeIDs())
	}
}

// TestTreeExcessNodes verifies that a configuration larger than the tree
// capacity is rejected (bf=3, depth=2, capacity=13; give 15 nodes).
func TestTreeExcessNodes(t *testing.T) {
	_, err := NewTree(makeTreeConfig(15), TreeOptions{BranchingFactor: 3, Depth: 2})
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
		id     ID
		wantID ID
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
			got := ID(0)
			if p != nil {
				got = p.ID()
			}
			t.Errorf("ParentOf(%d) = %d, want %d", tt.id, got, tt.wantID)
		}
	}

	// Subtree of node 2: {2, 4, 5, 8, 9, 10, 11}
	if got := tree.Subtree(2); !slices.Equal(got.NodeIDs(), []ID{2, 4, 5, 8, 9, 10, 11}) {
		t.Errorf("Subtree(2) = %v, want [2 4 5 8 9 10 11]", got.NodeIDs())
	}
}

// mustNewTree creates a TreeConfig for testing, failing the test on error.
func mustNewTree(t *testing.T, n int, opts TreeOptions) *TreeConfig {
	t.Helper()
	tree, err := NewTree(makeTreeConfig(n), opts)
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	return tree
}

// TestTreeOrderDefinesPositions verifies that positions come from the order of
// the configuration and not from node IDs, by building a tree over a permuted
// configuration (bf=2, depth=2, IDs in the order 5 3 7 1 6 2 4).
//
//	    5 (root)
//	   / \
//	  3   7
//	 / \ / \
//	1  6 2  4
func TestTreeOrderDefinesPositions(t *testing.T) {
	cfg := Config{}
	for _, id := range []ID{5, 3, 7, 1, 6, 2, 4} {
		cfg = append(cfg, makeTreeNode(id))
	}
	tree, err := NewTree(cfg, TreeOptions{BranchingFactor: 2, Depth: 2})
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	if got := tree.Root().ID(); got != 5 {
		t.Errorf("Root = %d, want 5", got)
	}
	if got := tree.ChildrenOf(5).NodeIDs(); !slices.Equal(got, []ID{3, 7}) {
		t.Errorf("ChildrenOf(5) = %v, want [3 7]", got)
	}
	if got := tree.ChildrenOf(3).NodeIDs(); !slices.Equal(got, []ID{1, 6}) {
		t.Errorf("ChildrenOf(3) = %v, want [1 6]", got)
	}
	if got := tree.ChildrenOf(7).NodeIDs(); !slices.Equal(got, []ID{2, 4}) {
		t.Errorf("ChildrenOf(7) = %v, want [2 4]", got)
	}
	if p := tree.ParentOf(2); p == nil || p.ID() != 7 {
		t.Errorf("ParentOf(2) = %v, want node 7", p)
	}
	if got := tree.Subtree(7).NodeIDs(); !slices.Equal(got, []ID{7, 2, 4}) {
		t.Errorf("Subtree(7) = %v, want [7 2 4]", got)
	}
	// Sorting the configuration by ID before building changes the shape.
	sorted, err := NewTree(cfg.Sort(ByID), TreeOptions{BranchingFactor: 2, Depth: 2})
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	if got := sorted.Root().ID(); got != 1 {
		t.Errorf("Root of sorted tree = %d, want 1", got)
	}
}

// TestTreeLevelHeightIsLeaf verifies Level, Height and IsLeaf on a partial
// tree (bf=3, depth=2, 11 nodes: node 4 has the single child 11).
//
//	     1
//	   / | \
//	  2  3  4
//	 /|\ /|\ |
//	5 6 7 8 9 10 11
func TestTreeLevelHeightIsLeaf(t *testing.T) {
	tree := mustNewTree(t, 11, TreeOptions{BranchingFactor: 3, Depth: 2})
	tests := []struct {
		id     ID
		level  int
		height int
		isLeaf bool
	}{
		{id: 1, level: 0, height: 2, isLeaf: false},
		{id: 2, level: 1, height: 1, isLeaf: false},
		{id: 4, level: 1, height: 1, isLeaf: false},
		{id: 5, level: 2, height: 0, isLeaf: true},
		{id: 11, level: 2, height: 0, isLeaf: true},
		{id: 99, level: -1, height: -1, isLeaf: false}, // not in tree
	}
	for _, tt := range tests {
		if got := tree.Level(tt.id); got != tt.level {
			t.Errorf("Level(%d) = %d, want %d", tt.id, got, tt.level)
		}
		if got := tree.Height(tt.id); got != tt.height {
			t.Errorf("Height(%d) = %d, want %d", tt.id, got, tt.height)
		}
		if got := tree.IsLeaf(tt.id); got != tt.isLeaf {
			t.Errorf("IsLeaf(%d) = %t, want %t", tt.id, got, tt.isLeaf)
		}
	}
	// A tree that does not fill its deepest level has a smaller height at
	// nodes without descendants there.
	shallow := mustNewTree(t, 4, TreeOptions{BranchingFactor: 3, Depth: 3})
	if got := shallow.Height(1); got != 1 {
		t.Errorf("Height(1) of a 4-node tree = %d, want 1", got)
	}
	if !shallow.IsLeaf(2) {
		t.Error("IsLeaf(2) = false, want true")
	}
}

// TestNewStar verifies that a star is a depth-1 tree with all other nodes as
// children of the root, including the degenerate sizes.
func TestNewStar(t *testing.T) {
	for _, n := range []int{1, 2, 3, 10} {
		tree, err := NewStar(makeTreeConfig(n))
		if err != nil {
			t.Fatalf("NewStar(%d nodes): %v", n, err)
		}
		if got := tree.Root().ID(); got != 1 {
			t.Errorf("n=%d: Root = %d, want 1", n, got)
		}
		if got := tree.ChildrenOf(1).Size(); got != n-1 {
			t.Errorf("n=%d: root has %d children, want %d", n, got, n-1)
		}
		for id := ID(2); id <= ID(n); id++ {
			if p := tree.ParentOf(id); p == nil || p.ID() != 1 {
				t.Errorf("n=%d: ParentOf(%d) = %v, want node 1", n, id, p)
			}
			if !tree.IsLeaf(id) {
				t.Errorf("n=%d: IsLeaf(%d) = false, want true", n, id)
			}
		}
	}
	if _, err := NewStar(nil); err == nil {
		t.Error("NewStar(nil) succeeded, want error")
	}
}

// TestTreeIsImmutable verifies that a tree is unaffected by later changes to
// the configuration it was built from, and that appending to a returned
// configuration cannot overwrite the tree's nodes.
func TestTreeIsImmutable(t *testing.T) {
	cfg := makeTreeConfig(7)
	tree := mustNewTree(t, 7, TreeOptions{BranchingFactor: 2, Depth: 2})
	tree2, err := NewTree(cfg, TreeOptions{BranchingFactor: 2, Depth: 2})
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	cfg[0] = makeTreeNode(100)
	if got := tree2.Root().ID(); got != 1 {
		t.Errorf("Root after changing the input = %d, want 1", got)
	}
	_ = append(tree.ChildrenOf(1), makeTreeNode(200))
	_ = append(tree.Config(), makeTreeNode(201))
	if got := tree.Config().NodeIDs(); !slices.Equal(got, []ID{1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("Config = %v after appends, want [1..7]", got)
	}
}

// TestTreeDuplicateNode verifies that a configuration listing a node twice is
// rejected, since a node would then have two positions.
func TestTreeDuplicateNode(t *testing.T) {
	cfg := Config{makeTreeNode(1), makeTreeNode(2), makeTreeNode(1)}
	_, err := NewTree(cfg, TreeOptions{BranchingFactor: 2, Depth: 2})
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("NewTree error = %v, want duplicate-node error", err)
	}
}
