package gorums

import (
	"fmt"
	"slices"
)

// TreeConfig is a tree topology over the nodes of a [Config]: a complete
// b-ary tree in breadth-first order, where b is the branching factor. In a
// complete tree, every level is full except possibly the last, which is
// filled from the left. The first node of the configuration is the root, the
// next b nodes are its children, the next b^2 nodes are the grandchildren,
// and so on. The tree has the smallest depth that holds all the nodes.
// A branching factor of 1 gives a chain.
//
// The order of the configuration alone defines the positions in the tree.
// Processes that relay over the same tree must build it from configurations
// in the same order.
//
// A TreeConfig is an immutable value; it is safe for concurrent use.
// To change the tree, for example to use a new tree in each view of a
// protocol, build a new TreeConfig or rotate it with [TreeConfig.Rotate].
// Old and new trees can be used at the same time.
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
	// nodes lists the tree in breadth-first order, and the position of a
	// node is its index in nodes; nodes[0] is the root. With arity a, the
	// children of position p are the positions p*a+1 through p*a+a that are
	// in nodes, and the parent of position p > 0 is (p-1)/a.
	// nodes is the window ring[offset:offset+len(nodes)].
	nodes Config
	// ring holds the node order given to NewTree twice in a row, so that
	// every rotation of it, and every range of positions in a rotation, is
	// a contiguous subslice. All rotations of a tree share ring.
	ring Config
	// offset is the rotation of nodes in ring, in the range [0, len(nodes)).
	offset int
	// branchingFactor is the branching factor given to NewTree.
	branchingFactor int
	// arity is min(branchingFactor, max(len(nodes)-1, 1)), the smallest
	// branching factor that gives the same tree.
	arity      int
	levels     []int      // levels[pos] is the level of the node at position pos
	positionOf map[ID]int // node ID → index in ring[:len(nodes)] (the unrotated tree)
}

// NewTree returns a [TreeConfig] over cfg with the given branching factor,
// the maximum number of children per node. The nodes are placed in the tree
// in the order of cfg, and the tree has the smallest depth that holds them.
// It returns an error if cfg is empty or if branchingFactor is less than 1.
func NewTree(cfg Config, branchingFactor int) (*TreeConfig, error) {
	if branchingFactor < 1 {
		return nil, fmt.Errorf("gorums: branching factor must be >= 1, got %d", branchingFactor)
	}
	if len(cfg) == 0 {
		return nil, fmt.Errorf("gorums: cannot build a tree from an empty configuration")
	}
	ring := slices.Concat(cfg, cfg)
	nodes := ring[:len(cfg):len(cfg)]
	positionOf := make(map[ID]int, len(nodes))
	for i, n := range nodes {
		if _, dup := positionOf[n.ID()]; dup {
			return nil, fmt.Errorf("gorums: configuration contains node %d more than once", n.ID())
		}
		positionOf[n.ID()] = i
	}
	// A tree with b >= len(nodes)-1 is a star for every such b, so the
	// arithmetic uses the smallest of them.
	arity := min(branchingFactor, max(len(nodes)-1, 1))
	levels := make([]int, len(nodes))
	for pos := 1; pos < len(nodes); pos++ {
		levels[pos] = levels[(pos-1)/arity] + 1
	}
	return &TreeConfig{nodes: nodes, ring: ring, branchingFactor: branchingFactor, arity: arity, levels: levels, positionOf: positionOf}, nil
}

// NewStar returns a [TreeConfig] in which the first node of cfg is the root
// and every other node is a child of the root. It is the tree that [NewTree]
// returns with a branching factor of max(len(cfg)-1, 1), and it is useful as
// a fallback when a deeper tree cannot be used.
// It returns an error if cfg is empty.
func NewStar(cfg Config) (*TreeConfig, error) {
	return NewTree(cfg, max(len(cfg)-1, 1))
}

// Rotate returns the tree with the breadth-first order of t rotated by r
// positions. A positive r rotates left, so the node r positions after the
// root of t becomes the root. A negative r rotates right.
// A rotation costs constant memory and time.
func (t *TreeConfig) Rotate(r int) *TreeConfig {
	n := len(t.nodes)
	rotated := *t
	rotated.offset = (t.offset + r%n + n) % n
	rotated.nodes = t.ring[rotated.offset : rotated.offset+n : rotated.offset+n]
	return &rotated
}

// position returns the position of the node with the given ID in the tree,
// and whether the node is in the tree.
func (t *TreeConfig) position(id ID) (int, bool) {
	pos, found := t.positionOf[id]
	if !found {
		return 0, false
	}
	if pos -= t.offset; pos < 0 {
		pos += len(t.nodes)
	}
	return pos, true
}

// BranchingFactor returns the branching factor that the tree was built with.
func (t *TreeConfig) BranchingFactor() int {
	return t.branchingFactor
}

// firstChild returns the position of the first child of position pos, or
// len(t.nodes) if the position has no children. It compares pos with
// (len(t.nodes)-2)/arity, the last position with a child, before it
// multiplies, so the result always fits in an int.
func (t *TreeConfig) firstChild(pos int) int {
	n := len(t.nodes)
	if n < 2 || pos > (n-2)/t.arity {
		return n
	}
	return pos*t.arity + 1
}

// ParentOf returns the parent of the node with the given ID, or nil if the
// node is the root or not in the tree.
func (t *TreeConfig) ParentOf(id ID) *Node {
	pos, found := t.position(id)
	if !found || pos == 0 {
		return nil
	}
	return t.nodes[(pos-1)/t.arity]
}

// PathTo returns the ancestors of the node with the given ID, root first,
// without the node itself. These are the nodes that a message relayed down
// the tree passes through on its way from the root to the node. It returns
// an empty Config for the root and nil for an ID not in the tree.
func (t *TreeConfig) PathTo(id ID) Config {
	pos, found := t.position(id)
	if !found {
		return nil
	}
	path := make(Config, t.levels[pos])
	for i := len(path) - 1; i >= 0; i-- {
		pos = (pos - 1) / t.arity
		path[i] = t.nodes[pos]
	}
	return path
}

// ChildrenOf returns the direct children of the node with the given ID as a
// Config. Returns nil for leaves and for IDs not in the tree.
// The returned configuration is a view of the tree's node list and must not
// be modified; appending to it is safe.
func (t *TreeConfig) ChildrenOf(id ID) Config {
	pos, found := t.position(id)
	if !found {
		return nil
	}
	first := t.firstChild(pos)
	if first >= len(t.nodes) {
		return nil
	}
	last := first + min(t.arity, len(t.nodes)-first)
	return t.nodes[first:last:last]
}

// Subtree returns a Config containing the given node and all its
// descendants in breadth-first order. Returns nil if the node is not in
// the tree.
func (t *TreeConfig) Subtree(id ID) Config {
	pos, found := t.position(id)
	if !found {
		return nil
	}
	// The descendants on each level are a contiguous range of positions.
	// Find the ranges first, so that the result is allocated at its exact
	// size and a leaf does not reserve the rest of the tree.
	type span struct{ start, end int }
	var spans []span
	size := 1
	for lo, hi := pos, pos+1; ; {
		lo, hi = t.firstChild(lo), min(t.firstChild(hi), len(t.nodes))
		if lo >= len(t.nodes) {
			break
		}
		spans = append(spans, span{lo, hi})
		size += hi - lo
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
	pos, found := t.position(id)
	if !found {
		return -1
	}
	return t.levels[pos]
}

// Height returns the number of edges on the longest path from the node with
// the given ID down to a leaf, so a leaf has height 0 and the root has the
// height of the tree. It returns -1 if the node is not in the tree.
func (t *TreeConfig) Height(id ID) int {
	pos, found := t.position(id)
	if !found {
		return -1
	}
	// The leftmost descendant is always on the deepest level of the subtree.
	h := 0
	for first := t.firstChild(pos); first < len(t.nodes); first = t.firstChild(first) {
		h++
	}
	return h
}

// IsLeaf reports whether the node with the given ID is in the tree and has
// no children.
func (t *TreeConfig) IsLeaf(id ID) bool {
	_, found := t.position(id)
	return found && len(t.ChildrenOf(id)) == 0
}
