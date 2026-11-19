package gorums

import (
	"fmt"
	"math"
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
		name            string
		cfg             Config
		branchingFactor int
		wantErr         string
	}{
		{name: "BranchingFactorZero", cfg: cfg, branchingFactor: 0, wantErr: "branching factor must be >= 1"},
		{name: "BranchingFactorNegative", cfg: cfg, branchingFactor: -1, wantErr: "branching factor must be >= 1"},
		{name: "EmptyConfig", cfg: Config{}, branchingFactor: 2, wantErr: "empty configuration"},
		{name: "NilConfig", cfg: nil, branchingFactor: 2, wantErr: "empty configuration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTree(tt.cfg, tt.branchingFactor)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestNewTreeDepth verifies that a tree has the smallest depth that holds
// all the nodes of the configuration.
func TestNewTreeDepth(t *testing.T) {
	tests := []struct {
		n, branchingFactor, height int
	}{
		{n: 1, branchingFactor: 1, height: 0},
		{n: 1, branchingFactor: 5, height: 0},
		{n: 2, branchingFactor: 2, height: 1},
		{n: 3, branchingFactor: 2, height: 1},
		{n: 4, branchingFactor: 2, height: 2},
		{n: 7, branchingFactor: 2, height: 2},
		{n: 8, branchingFactor: 2, height: 3},
		{n: 13, branchingFactor: 3, height: 2},
		{n: 14, branchingFactor: 3, height: 3},
		{n: 5, branchingFactor: 1, height: 4},
		{n: 5, branchingFactor: math.MaxInt, height: 1},
	}
	for _, tt := range tests {
		tree := mustNewTree(t, tt.n, tt.branchingFactor)
		if got := tree.Height(1); got != tt.height {
			t.Errorf("n=%d, b=%d: Height(root) = %d, want %d", tt.n, tt.branchingFactor, got, tt.height)
		}
		if got := tree.BranchingFactor(); got != tt.branchingFactor {
			t.Errorf("n=%d, b=%d: BranchingFactor() = %d", tt.n, tt.branchingFactor, got)
		}
	}
}

// TestTreeChain verifies that a branching factor of 1 builds a chain in the
// order of the configuration.
//
//	1 - 2 - 3 - 4
func TestTreeChain(t *testing.T) {
	tree := mustNewTree(t, 4, 1)
	for id := ID(1); id <= 4; id++ {
		if got := tree.Level(id); got != int(id)-1 {
			t.Errorf("Level(%d) = %d, want %d", id, got, id-1)
		}
		var wantChildren []ID
		if id < 4 {
			wantChildren = []ID{id + 1}
		}
		if got := tree.ChildrenOf(id).NodeIDs(); !slices.Equal(got, wantChildren) {
			t.Errorf("ChildrenOf(%d) = %v, want %v", id, got, wantChildren)
		}
	}
	if p := tree.ParentOf(4); p == nil || p.ID() != 3 {
		t.Errorf("ParentOf(4) = %v, want node 3", p)
	}
	if got := tree.Subtree(2).NodeIDs(); !slices.Equal(got, []ID{2, 3, 4}) {
		t.Errorf("Subtree(2) = %v, want [2 3 4]", got)
	}
}

// TestTreeArithmeticOverflow verifies the position methods on a tree whose
// child positions overflow int, as they can for a large tree on a 32-bit
// platform. The tree is built by hand, since a configuration large enough to
// overflow on a 64-bit platform does not fit in memory.
//
//	  1
//	 / \
//	2   3
func TestTreeArithmeticOverflow(t *testing.T) {
	tree := &TreeConfig{
		nodes:      makeTreeConfig(3),
		arity:      math.MaxInt,
		levels:     []int{0, 1, 1},
		positionOf: map[ID]int{1: 0, 2: 1, 3: 2},
	}
	if got := tree.Level(3); got != 1 {
		t.Errorf("Level(3) = %d, want 1", got)
	}
	if p := tree.ParentOf(3); p == nil || p.ID() != 1 {
		t.Errorf("ParentOf(3) = %v, want node 1", p)
	}
	if got := tree.ChildrenOf(1).NodeIDs(); !slices.Equal(got, []ID{2, 3}) {
		t.Errorf("ChildrenOf(1) = %v, want [2 3]", got)
	}
	if got := tree.ChildrenOf(3); got != nil {
		t.Errorf("ChildrenOf(3) = %v, want nil", got.NodeIDs())
	}
	if got := tree.Subtree(1).NodeIDs(); !slices.Equal(got, []ID{1, 2, 3}) {
		t.Errorf("Subtree(1) = %v, want [1 2 3]", got)
	}
	if got := tree.Height(1); got != 1 {
		t.Errorf("Height(1) = %d, want 1", got)
	}
}

// TestTreeParentOf verifies ParentOf on a perfect bf=3 depth=2 tree.
func TestTreeParentOf(t *testing.T) {
	tree := mustNewTree(t, 13, 3)
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
	tree := mustNewTree(t, 13, 3)
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

// TestTreePathTo verifies PathTo on a perfect bf=3 depth=2 tree.
func TestTreePathTo(t *testing.T) {
	tree := mustNewTree(t, 13, 3)
	tests := []struct {
		id      ID
		wantIDs []ID
	}{
		{id: 1, wantIDs: []ID{}}, // root: empty, not nil
		{id: 2, wantIDs: []ID{1}},
		{id: 4, wantIDs: []ID{1}},
		{id: 5, wantIDs: []ID{1, 2}},
		{id: 10, wantIDs: []ID{1, 3}},
		{id: 13, wantIDs: []ID{1, 4}},
		{id: 14, wantIDs: nil}, // not in tree
	}
	for _, tt := range tests {
		got := tree.PathTo(tt.id)
		if (got == nil) != (tt.wantIDs == nil) || !slices.Equal(got.NodeIDs(), tt.wantIDs) {
			t.Errorf("PathTo(%d) = %v (nil=%t), want %v (nil=%t)", tt.id, got.NodeIDs(), got == nil, tt.wantIDs, tt.wantIDs == nil)
		}
	}
	chain := mustNewTree(t, 5, 1)
	if got := chain.PathTo(5).NodeIDs(); !slices.Equal(got, []ID{1, 2, 3, 4}) {
		t.Errorf("chain PathTo(5) = %v, want [1 2 3 4]", got)
	}
}

// TestTreeSubtree verifies Subtree on a perfect bf=3 depth=2 tree.
func TestTreeSubtree(t *testing.T) {
	tree := mustNewTree(t, 13, 3)
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
	tree := mustNewTree(t, 10, 3)

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
	tree := mustNewTree(t, 12, 3)
	// Node 4 (level 1, idx 2): children at 11, 12 — position 12 (slot for ID 13) absent.
	if got := tree.ChildrenOf(4); !slices.Equal(got.NodeIDs(), []ID{11, 12}) {
		t.Errorf("ChildrenOf(4) = %v, want [11 12]", got.NodeIDs())
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
	tree := mustNewTree(t, 15, 2)

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
func mustNewTree(t *testing.T, n, branchingFactor int) *TreeConfig {
	t.Helper()
	tree, err := NewTree(makeTreeConfig(n), branchingFactor)
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
	tree, err := NewTree(cfg, 2)
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
	sorted, err := NewTree(cfg.Sort(ByID), 2)
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
	tree := mustNewTree(t, 11, 3)
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
	// A node above the last level without children of its own is a leaf.
	partial := mustNewTree(t, 10, 3)
	if got := partial.Height(4); got != 0 {
		t.Errorf("Height(4) of a 10-node tree = %d, want 0", got)
	}
	if !partial.IsLeaf(4) {
		t.Error("IsLeaf(4) = false, want true")
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
	tree := mustNewTree(t, 7, 2)
	tree2, err := NewTree(cfg, 2)
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
	_, err := NewTree(cfg, 2)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("NewTree error = %v, want duplicate-node error", err)
	}
}

// TestTreeSubtreeAllocation verifies that a subtree is allocated at its exact
// size, so that the subtree of a leaf in a large star does not reserve room
// for the rest of the configuration.
func TestTreeSubtreeAllocation(t *testing.T) {
	star, err := NewStar(makeTreeConfig(1000))
	if err != nil {
		t.Fatalf("NewStar: %v", err)
	}
	if got := cap(star.Subtree(2)); got != 1 {
		t.Errorf("cap(Subtree(leaf)) = %d, want 1", got)
	}
	tree := mustNewTree(t, 13, 3)
	for _, id := range []ID{1, 2, 3, 4, 5, 13} {
		if sub := tree.Subtree(id); cap(sub) != len(sub) {
			t.Errorf("Subtree(%d): cap = %d, len = %d, want equal", id, cap(sub), len(sub))
		}
	}
}

// BenchmarkTreePositions measures the position lookups that a relay makes
// once per message, on a 100-node binary tree and a 100-node chain.
func BenchmarkTreePositions(b *testing.B) {
	for _, bf := range []int{2, 1} {
		tree, err := NewTree(makeTreeConfig(100), bf)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("b=%d", bf), func(b *testing.B) {
			for b.Loop() {
				for id := ID(1); id <= 100; id++ {
					_ = tree.ParentOf(id)
					_ = tree.ChildrenOf(id)
					_ = tree.Level(id)
				}
			}
		})
	}
}
