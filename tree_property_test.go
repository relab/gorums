package gorums

import (
	"math"
	"math/big"
	"slices"
	"testing"

	"pgregory.net/rapid"
)

func TestTreeProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ids := rapid.SliceOfNDistinct(rapid.Uint32Range(1, 10000), 1, 100, rapid.ID).Draw(t, "ids")
		bf := rapid.IntRange(1, 12).Draw(t, "branchingFactor")
		cfg := make(Config, len(ids))
		for i, id := range ids {
			cfg[i] = makeTreeNode(id)
		}
		// Construct an adjacency list in breadth-first order. Expected topology is
		// obtained by traversal rather than the implementation's level arithmetic.
		children := make([][]int, len(ids))
		parents := make([]int, len(ids))
		levels := make([]int, len(ids))
		parents[0] = -1
		next := 1
		for parent := 0; next < len(ids); parent++ {
			for range bf {
				if next == len(ids) {
					break
				}
				children[parent] = append(children[parent], next)
				parents[next], levels[next] = parent, levels[parent]+1
				next++
			}
		}
		tree, err := NewTree(cfg, bf)
		if err != nil {
			t.Fatal(err)
		}
		if tree.BranchingFactor() != bf {
			t.Fatalf("BranchingFactor() = %d, want %d", tree.BranchingFactor(), bf)
		}
		for i, id := range ids {
			var parent *Node
			if parents[i] >= 0 {
				parent = cfg[parents[i]]
			}
			if tree.ParentOf(id) != parent || tree.Level(id) != levels[i] {
				t.Fatalf("parent/level mismatch for %d", id)
			}
			var wantChildren []ID
			for _, child := range children[i] {
				wantChildren = append(wantChildren, ids[child])
			}
			if !slices.Equal(tree.ChildrenOf(id).NodeIDs(), wantChildren) {
				t.Fatalf("children mismatch for %d", id)
			}
			var wantPath []ID
			for p := parents[i]; p >= 0; p = parents[p] {
				wantPath = append([]ID{ids[p]}, wantPath...)
			}
			if path := tree.PathTo(id); path == nil || !slices.Equal(path.NodeIDs(), wantPath) {
				t.Fatalf("path mismatch for %d: got %v, want %v", id, path.NodeIDs(), wantPath)
			}
			queue := []int{i}
			var subtree []ID
			height := 0
			for len(queue) > 0 {
				node := queue[0]
				queue = queue[1:]
				subtree = append(subtree, ids[node])
				height = max(height, levels[node]-levels[i])
				queue = append(queue, children[node]...)
			}
			if !slices.Equal(tree.Subtree(id).NodeIDs(), subtree) || tree.Height(id) != height || tree.IsLeaf(id) != (len(children[i]) == 0) {
				t.Fatalf("subtree/height/leaf mismatch for %d", id)
			}
		}
		if tree.ParentOf(0) != nil || tree.PathTo(0) != nil || len(tree.ChildrenOf(0)) != 0 || len(tree.Subtree(0)) != 0 || tree.Level(0) != -1 || tree.Height(0) != -1 || tree.IsLeaf(0) {
			t.Fatal("unknown node has a tree position")
		}
		star, err := NewStar(cfg)
		if err != nil || star.Root() != cfg[0] || !slices.Equal(star.ChildrenOf(ids[0]).NodeIDs(), ids[1:]) {
			t.Fatalf("star: %v", err)
		}
		duplicate := append(slices.Clone(cfg), cfg[0])
		if _, err := NewStar(duplicate); err == nil {
			t.Fatal("duplicate ID accepted")
		}
		cfg[0] = nil
		if !slices.Equal(tree.Config().NodeIDs(), ids) || tree.Root().ID() != ids[0] {
			t.Fatal("tree aliases source slice")
		}
	})
}

func FuzzNewTree(f *testing.F) {
	for _, shape := range [][2]int{{2, 3}, {2, 7}, {2, 8}, {1, 1}, {1, 255}, {0, 1}, {math.MaxInt, 3}, {math.MinInt, 1}, {-1, 0}} {
		f.Add(shape[0], uint8(shape[1]))
	}
	f.Fuzz(func(t *testing.T, bf int, count uint8) {
		n := int(count)
		cfg := makeTreeConfig(n)
		tree, err := NewTree(cfg, bf)
		if valid := bf >= 1 && n > 0; (err == nil) != valid {
			t.Fatalf("NewTree(b=%d,n=%d) = %v, valid=%v", bf, n, err, valid)
		}
		if err != nil {
			return
		}
		// The oracle finds the smallest depth whose capacity
		// (b^(depth+1) − 1) / (b − 1) holds n nodes, in exact arithmetic.
		depth := 0
		capacity, power := big.NewInt(1), big.NewInt(1)
		for capacity.Cmp(big.NewInt(int64(n))) < 0 {
			power.Mul(power, big.NewInt(int64(bf)))
			capacity.Add(capacity, power)
			depth++
		}
		if tree.Root() != cfg[0] || !slices.Equal(tree.Subtree(cfg[0].ID()), cfg) {
			t.Fatal("root subtree differs from configuration")
		}
		if got := tree.Height(cfg[0].ID()); got != depth {
			t.Fatalf("NewTree(b=%d,n=%d): Height(root) = %d, want %d", bf, n, got, depth)
		}
		if got := tree.Level(cfg[n-1].ID()); got != depth {
			t.Fatalf("NewTree(b=%d,n=%d): Level(last) = %d, want %d", bf, n, got, depth)
		}
	})
}
