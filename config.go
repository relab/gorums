package gorums

import (
	"sort"

	"github.com/relab/gorums/ordering"
)

// Configuration represents a static set of nodes on which quorum calls may be invoked.
type Configuration []*Node

func NewConfiguration(mgr *Manager, ids []uint32) (Configuration, error) {
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
	return nodes, nil
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
func (c Configuration) Nodes() []*Node {
	return c
}

// Size returns the number of nodes in this configuration.
func (c Configuration) Size() int {
	return len(c)
}

// newCall returns unique metadata for a method call.
func (c Configuration) newCall(method string) (md *ordering.Metadata) {
	// Note that we just use the first node's newCall method since all nodes
	// associated with the same manager use the same receiveQueue instance.
	return c[0].newCall(method)
}

// newReply returns a channel for receiving replies
// and a done function to be called for clean up.
func (c Configuration) newReply(md *ordering.Metadata, maxReplies int) (replyChan chan *gorumsStreamResult, done func()) {
	// Note that we just use the first node's newReply method since all nodes
	// associated with the same manager use the same receiveQueue instance.
	return c[0].newReply(md, maxReplies)
}
