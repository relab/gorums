package gorums

import (
	"sort"
)

// Configuration represents a static set of nodes on which quorum calls may be invoked.
type Configuration struct {
	nodes []*Node
	errs  chan Error
}

func NewConfiguration(mgr *Manager, ids []uint32) (*Configuration, error) {
	if len(ids) == 0 {
		return nil, IllegalConfigError("need at least one node")
	}

	var nodes []*Node
	unique := make(map[uint32]struct{})
	for _, nid := range ids {
		// ensure that identical IDs are only counted once
		if _, duplicate := unique[nid]; duplicate {
			continue
		}
		unique[nid] = struct{}{}

		node, found := mgr.Node(nid)
		if !found {
			return nil, NodeNotFoundError(nid)
		}

		i := sort.Search(len(nodes), func(i int) bool {
			return node.ID() < nodes[i].ID()
		})
		nodes = append(nodes, nil)
		copy(nodes[i+1:], nodes[i:])
		nodes[i] = node
	}

	c := &Configuration{
		nodes: nodes,
	}
	return c, nil
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c *Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c.nodes))
	for i, node := range c.nodes {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
func (c *Configuration) Nodes() []*Node {
	return c.nodes
}

// Node returns a new configuration with a single node with the given identifier, if present.
func (c *Configuration) Node(id uint32) (cfg *Configuration, found bool) {
	for i, n := range c.nodes {
		if n.id == id {
			return &Configuration{nodes: []*Node{c.nodes[i]}}, true
		}
	}
	return nil, false
}

// Size returns the number of nodes in this configuration.
func (c *Configuration) Size() int {
	return len(c.nodes)
}

// SubError returns a channel for listening to individual node errors. Currently
// only a single listener is supported.
func (c *Configuration) SubError() <-chan Error {
	return c.errs
}
