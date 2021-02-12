package gorums

import (
	"fmt"

	"github.com/relab/gorums/ordering"
)

// Configuration represents a static set of nodes on which quorum calls may be invoked.
type Configuration []*Node

// NewConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList.
func NewConfiguration(mgr *Manager, opts ...ConfigOption) (nodes Configuration, err error) {
	o := newConfigOptions()
	for _, opt := range opts {
		opt(&o)
	}

	nodes = make(Configuration, 0)
	switch {
	case len(o.addrsList) == 0 && len(o.idMapping) == 0 && len(o.nodeIDs) == 0:
		return nil, ConfigCreationError(fmt.Errorf("no nodes provided; need WithNodeMap or WithNodeList or WithNodeIDs"))

	case len(o.addrsList) > 0 && len(o.idMapping) > 0:
		return nil, ConfigCreationError(fmt.Errorf("multiple node lists provided; use only one of WithNodeMap or WithNodeList"))
	case len(o.addrsList) > 0 && len(o.nodeIDs) > 0:
		return nil, ConfigCreationError(fmt.Errorf("multiple node lists provided; use only one of WithNodeList or WithNodeIDs"))
	case len(o.idMapping) > 0 && len(o.nodeIDs) > 0:
		return nil, ConfigCreationError(fmt.Errorf("multiple node lists provided; use only one of WithNodeMap or WithNodeIDs"))

	case len(o.idMapping) > 0:
		for naddr, id := range o.idMapping {
			node, found := mgr.Node(id)
			if !found {
				node, err = NewNodeWithID(naddr, id)
				if err != nil {
					return nil, ConfigCreationError(err)
				}
				err = mgr.AddNode(node)
				if err != nil {
					return nil, ConfigCreationError(err)
				}
			}
			nodes = append(nodes, node)
		}

	case len(o.addrsList) > 0:
		for _, naddr := range o.addrsList {
			node, err := NewNode(naddr)
			if err != nil {
				return nil, ConfigCreationError(err)
			}
			if n, found := mgr.Node(node.ID()); !found {
				err = mgr.AddNode(node)
				if err != nil {
					return nil, ConfigCreationError(err)
				}
			} else {
				node = n
			}
			nodes = append(nodes, node)
		}

	case len(o.nodeIDs) > 0:
		for _, id := range o.nodeIDs {
			node, found := mgr.Node(id)
			if !found {
				// Node IDs must have been registered previously
				return nil, ConfigCreationError(fmt.Errorf("node ID %d not found", id))
			}
			nodes = append(nodes, node)
		}
	}

	// Sort nodes to ensure deterministic iteration.
	OrderedBy(ID).Sort(mgr.nodes)
	OrderedBy(ID).Sort(nodes)
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

// Equal returns true if configurations b and c have the same set of nodes.
func (c Configuration) Equal(b Configuration) bool {
	if len(c) != len(b) {
		return false
	}
	for i := range c {
		if c[i].ID() != b[i].ID() {
			return false
		}
	}
	return true
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
