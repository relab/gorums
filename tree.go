package gorums

import (
	"context"
	"fmt"
	"math"
	"math/bits"
)

// TreeOptions configures the shape of a TreeConfiguration.
type TreeOptions struct {
	BranchingFactor int // number of children per internal node; must be >= 2
	Depth           int // number of edge-hops from root to leaves; must be >= 1
}

// TreeConfiguration is a tree-shaped view over a flat Configuration.
// The node at index 0 is the root; the next BranchingFactor indices are its
// children; the next BranchingFactor^2 indices are the grandchildren; and so on
// (breadth-first order). If the configuration has fewer nodes than a perfect
// tree of the given shape, the last level is partial.
//
// Use [Configuration.AsTree] to construct a TreeConfiguration.
type TreeConfiguration struct {
	nodes      Configuration // breadth-first order; nodes[0] is the root
	opts       TreeOptions
	positionOf map[uint32]int // node ID → index in nodes
}

// AsTree builds a TreeConfiguration from cfg using the given options.
// Nodes are placed in tree order by their position in cfg.
// Returns an error if cfg is empty, the options are invalid, or cfg has more
// nodes than the tree capacity ((bf^(depth+1) − 1) / (bf − 1)).
// If cfg has fewer nodes than capacity, the last level is partial.
func (cfg Configuration) AsTree(opts TreeOptions) (*TreeConfiguration, error) {
	if opts.BranchingFactor < 2 {
		return nil, fmt.Errorf("gorums: TreeOptions.BranchingFactor must be >= 2, got %d", opts.BranchingFactor)
	}
	if opts.Depth < 1 {
		return nil, fmt.Errorf("gorums: TreeOptions.Depth must be >= 1, got %d", opts.Depth)
	}
	if len(cfg) == 0 {
		return nil, fmt.Errorf("gorums: cannot build a tree from an empty configuration")
	}
	capacity := treeLevelStart(opts.Depth+1, opts.BranchingFactor)
	if capacity < 0 {
		return nil, fmt.Errorf("gorums: tree shape (BranchingFactor=%d, Depth=%d) exceeds representable range", opts.BranchingFactor, opts.Depth)
	}
	if len(cfg) > capacity {
		return nil, fmt.Errorf("gorums: configuration has %d nodes, exceeds tree capacity of %d", len(cfg), capacity)
	}
	positionOf := make(map[uint32]int, len(cfg))
	for i, n := range cfg {
		positionOf[n.ID()] = i
	}
	return &TreeConfiguration{
		nodes:      cfg,
		opts:       opts,
		positionOf: positionOf,
	}, nil
}

// treeLevelStart returns the index of the first node at level k in a tree with
// the given branching factor (bf > 1), i.e., the total number of nodes in
// levels 0 through k−1, computed as the sum bf^0 + bf^1 + … + bf^(k−1).
// Returns -1 if the result would overflow int; callers at validation boundaries
// must check for -1 before using the result.
func treeLevelStart(k, bf int) int {
	sum, pow := uint(0), uint(1)
	for i := range k {
		var carry uint
		sum, carry = bits.Add(sum, pow, 0)
		// ensure sum still fits in an int value before the final int(sum) conversion
		if carry != 0 || sum > uint(math.MaxInt) {
			return -1
		}
		if i+1 < k {
			hi, lo := bits.Mul(pow, uint(bf))
			if hi != 0 {
				return -1
			}
			pow = lo
		}
	}
	return int(sum)
}

// treePow returns base^exp for non-negative exp.
func treePow(base, exp int) int {
	result := 1
	for range exp {
		result *= base
	}
	return result
}

// posLevel returns the depth and within-level index of the node at position pos.
// Returns -1, -1 if pos is out of range.
func (t *TreeConfiguration) posLevel(pos int) (depth, indexInLevel int) {
	if pos < 0 || pos >= len(t.nodes) {
		return -1, -1
	}
	bf := t.opts.BranchingFactor
	for d := range t.opts.Depth + 1 {
		if pos < treeLevelStart(d+1, bf) {
			return d, pos - treeLevelStart(d, bf)
		}
	}
	return -1, -1
}

// ParentOf returns the parent of the node with the given ID, or nil if the
// node is the root or not in the tree.
func (t *TreeConfiguration) ParentOf(id uint32) *Node {
	pos, found := t.positionOf[id]
	if !found {
		return nil
	}
	d, idx := t.posLevel(pos)
	if d <= 0 {
		return nil
	}
	parentPos := treeLevelStart(d-1, t.opts.BranchingFactor) + idx/t.opts.BranchingFactor
	return t.nodes[parentPos]
}

// ChildrenOf returns the direct children of the node with the given ID as a
// Configuration. Returns nil for leaves and for IDs not in the tree.
func (t *TreeConfiguration) ChildrenOf(id uint32) Configuration {
	pos, found := t.positionOf[id]
	if !found {
		return nil
	}
	d, idx := t.posLevel(pos)
	if d < 0 || d >= t.opts.Depth {
		return nil
	}
	bf := t.opts.BranchingFactor
	firstChild := treeLevelStart(d+1, bf) + idx*bf
	if firstChild >= len(t.nodes) {
		return nil
	}
	lastChild := min(firstChild+bf, len(t.nodes))
	return t.nodes[firstChild:lastChild]
}

// Subtree returns a Configuration containing the given node and all its
// descendants in breadth-first order. Returns nil if the node is not in
// the tree.
func (t *TreeConfiguration) Subtree(id uint32) Configuration {
	pos, found := t.positionOf[id]
	if !found {
		return nil
	}
	d, idx := t.posLevel(pos)
	if d < 0 {
		return nil
	}
	bf := t.opts.BranchingFactor
	result := make(Configuration, 0, len(t.nodes)-pos)
	result = append(result, t.nodes[pos])
	for lvl := d + 1; lvl <= t.opts.Depth; lvl++ {
		span := treePow(bf, lvl-d)
		subtreeStart := treeLevelStart(lvl, bf) + idx*span
		if subtreeStart >= len(t.nodes) {
			break
		}
		subtreeEnd := min(subtreeStart+span, len(t.nodes))
		result = append(result, t.nodes[subtreeStart:subtreeEnd]...)
	}
	return result
}

// Context returns a ConfigContext for use with generated tree-call client
// wrappers, addressed to the direct children of the tree root. The client acts
// as the external root; generated server-side dispatchers handle further relay
// via [ServerCtx.TreeChildren].
//
// Panics if the root has no children (e.g., a single-node configuration).
func (t *TreeConfiguration) Context(parent context.Context) *ConfigContext {
	children := t.ChildrenOf(t.nodes[0].ID())
	if len(children) == 0 {
		panic("gorums: tree root has no children")
	}
	return children.Context(parent)
}
