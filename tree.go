package gorums

import (
	"fmt"
	"math"
	"math/bits"
	"slices"
)

// TreeOptions configures the shape of a [TreeConfig].
type TreeOptions struct {
	BranchingFactor int // maximum number of children per node; must be >= 2
	Depth           int // maximum number of edges from the root to a leaf; must be >= 1
}

// TreeConfig is a tree topology over the nodes of a [Config]: a complete
// b-ary tree in breadth-first order. The first node of the configuration is
// the root, the next BranchingFactor nodes are its children, the next
// BranchingFactor^2 nodes are the grandchildren, and so on. If the
// configuration has fewer nodes than a full tree of the given shape, the last
// level is partially filled from the left.
//
// The order of the configuration alone defines the positions in the tree.
// Node IDs have no influence. To control which node is the root, and which
// nodes share a parent, order the configuration first, for example with
// [Config.Sort], and then call [NewTree].
//
// A TreeConfig is an immutable value; it is safe for concurrent use.
// To change the tree, for example to use a new tree in each view of a
// protocol, build a new TreeConfig. Old and new trees can be used at the same
// time, since they only share the underlying nodes.
//
// A TreeConfig describes topology only. It does not reduce the number of
// connections: every node of the configuration is connected as soon as the
// configuration is created, whether or not the node has a place in the call
// path of its own tree position. A tree reduces the load on the sender,
// because it addresses the children of its own position instead of all the
// nodes, but all connections stay open.
//
// The root is a node like any other and may be a replica that takes part in
// the protocol. A caller outside the configuration starts a tree call by
// sending to the root, so that the root does its own work and relays to its
// children; sending to the root's children instead would skip the root. The
// process at the root itself sends to [TreeConfig.ChildrenOf] its own ID after
// its local work.
type TreeConfig struct {
	nodes      Config // breadth-first order; nodes[0] is the root
	opts       TreeOptions
	positionOf map[ID]int // node ID → index in nodes
}

// NewTree returns a [TreeConfig] over cfg using the given options.
// The nodes are placed in the tree in the order of cfg.
// It returns an error if cfg is empty, the options are invalid, or cfg has
// more nodes than the tree can hold, which is (b^(depth+1) − 1) / (b − 1)
// for branching factor b. If cfg has fewer nodes, the last level is partial.
//
// The tree keeps its own copy of the node list, so later changes to cfg
// do not affect it.
func NewTree(cfg Config, opts TreeOptions) (*TreeConfig, error) {
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
	nodes := slices.Clone(cfg)
	positionOf := make(map[ID]int, len(nodes))
	for i, n := range nodes {
		if _, dup := positionOf[n.ID()]; dup {
			return nil, fmt.Errorf("gorums: configuration contains node %d more than once", n.ID())
		}
		positionOf[n.ID()] = i
	}
	return &TreeConfig{nodes: nodes, opts: opts, positionOf: positionOf}, nil
}

// NewStar returns a [TreeConfig] in which the first node of cfg is the root
// and every other node is a child of the root. It is a tree of depth 1 whose
// branching factor is the number of other nodes, and it is useful as a
// fallback when a deeper tree cannot be used.
// It returns an error if cfg is empty.
func NewStar(cfg Config) (*TreeConfig, error) {
	return NewTree(cfg, TreeOptions{BranchingFactor: max(len(cfg)-1, 2), Depth: 1})
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
func (t *TreeConfig) posLevel(pos int) (depth, indexInLevel int) {
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
func (t *TreeConfig) ParentOf(id ID) *Node {
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
// Config. Returns nil for leaves and for IDs not in the tree.
func (t *TreeConfig) ChildrenOf(id ID) Config {
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
	return t.nodes[firstChild:lastChild:lastChild]
}

// Subtree returns a Config containing the given node and all its
// descendants in breadth-first order. Returns nil if the node is not in
// the tree.
func (t *TreeConfig) Subtree(id ID) Config {
	pos, found := t.positionOf[id]
	if !found {
		return nil
	}
	d, idx := t.posLevel(pos)
	if d < 0 {
		return nil
	}
	bf := t.opts.BranchingFactor
	// Find the descendant ranges first, so that the result is allocated at
	// its exact size and a leaf does not reserve the rest of the tree.
	type span struct{ start, end int }
	var spans []span
	size := 1
	for lvl := d + 1; lvl <= t.opts.Depth; lvl++ {
		width := treePow(bf, lvl-d)
		start := treeLevelStart(lvl, bf) + idx*width
		if start >= len(t.nodes) {
			break
		}
		end := min(start+width, len(t.nodes))
		spans = append(spans, span{start, end})
		size += end - start
	}
	result := make(Config, 0, size)
	result = append(result, t.nodes[pos])
	for _, s := range spans {
		result = append(result, t.nodes[s.start:s.end]...)
	}
	return result
}

// Config returns the configuration the tree is built over, in tree order.
// The root is the first node. The returned configuration must not be modified.
func (t *TreeConfig) Config() Config {
	return slices.Clip(t.nodes)
}

// Root returns the root of the tree.
func (t *TreeConfig) Root() *Node {
	return t.nodes[0]
}

// Level returns the number of edges between the node with the given ID and
// the root, so the root is at level 0. It returns -1 if the node is not in
// the tree.
func (t *TreeConfig) Level(id ID) int {
	pos, found := t.positionOf[id]
	if !found {
		return -1
	}
	d, _ := t.posLevel(pos)
	return d
}

// Height returns the number of edges on the longest path from the node with
// the given ID down to a leaf, so a leaf has height 0 and the root has the
// height of the tree. It returns -1 if the node is not in the tree.
func (t *TreeConfig) Height(id ID) int {
	pos, found := t.positionOf[id]
	if !found {
		return -1
	}
	// The leftmost descendant is always on the deepest level of the subtree.
	bf := t.opts.BranchingFactor
	h := 0
	for {
		d, idx := t.posLevel(pos)
		first := treeLevelStart(d+1, bf) + idx*bf
		if d >= t.opts.Depth || first >= len(t.nodes) {
			return h
		}
		pos = first
		h++
	}
}

// IsLeaf reports whether the node with the given ID is in the tree and has
// no children.
func (t *TreeConfig) IsLeaf(id ID) bool {
	_, found := t.positionOf[id]
	return found && len(t.ChildrenOf(id)) == 0
}
