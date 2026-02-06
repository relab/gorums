package gorums

import (
	"fmt"
	"slices"
)

// NodeListOption must be implemented by node providers.
type NodeListOption interface {
	Option
	newConfig(*Manager) (Configuration, error)
}

// NodeAddress must be implemented by types that can be used as node addresses.
type NodeAddress interface {
	Addr() string
}

type structNodeMap[T NodeAddress] struct {
	nodes map[uint32]T
}

func (structNodeMap[T]) isOption() {}

func (o structNodeMap[T]) newConfig(mgr *Manager) (nodes Configuration, err error) {
	if len(o.nodes) == 0 {
		return nil, fmt.Errorf("config: missing required node map")
	}
	nodes = make(Configuration, 0, len(o.nodes))
	for id, n := range o.nodes {
		if id == 0 {
			return nil, fmt.Errorf("config: node ID 0 is reserved; use IDs > 0")
		}
		node, found := mgr.Node(id)
		if !found {
			node, err = mgr.newNode(n.Addr(), id)
			if err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, node)
	}
	// Sort nodes to ensure deterministic iteration.
	OrderedBy(ID).Sort(mgr.nodes)
	OrderedBy(ID).Sort(nodes)
	return nodes, nil
}

// WithNodes returns a NodeListOption containing the provided mapping from
// application-specific IDs to types implementing NodeAddress.
// Node IDs must be greater than 0.
func WithNodes[T NodeAddress](nodes map[uint32]T) NodeListOption {
	return &structNodeMap[T]{nodes: nodes}
}

type nodeList struct {
	addrsList []string
}

func (nodeList) isOption() {}

func (o nodeList) newConfig(mgr *Manager) (nodes Configuration, err error) {
	if len(o.addrsList) == 0 {
		return nil, fmt.Errorf("config: missing required node addresses")
	}
	// Find the highest ID currently in use; IDs start from max(nodeIDs) + 1
	// If manager is empty, maxID is 0, so new IDs start from 1
	maxID := uint32(0)
	if nodeIDs := mgr.NodeIDs(); len(nodeIDs) > 0 {
		maxID = slices.Max(nodeIDs)
	}
	nodes = make(Configuration, 0, len(o.addrsList))
	for i, naddr := range o.addrsList {
		id := maxID + uint32(i) + 1
		node, err := mgr.newNode(naddr, id)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	// Sort nodes to ensure deterministic iteration.
	OrderedBy(ID).Sort(mgr.nodes)
	OrderedBy(ID).Sort(nodes)
	return nodes, nil
}

// WithNodeList returns a NodeListOption for the provided list of node addresses.
// Unique Node IDs are generated preventing conflicts with existing nodes.
func WithNodeList(addrsList []string) NodeListOption {
	return &nodeList{addrsList: addrsList}
}
