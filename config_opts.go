package gorums

import (
	"fmt"
	"maps"
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
	// Build a map of addresses to IDs for the new nodes
	addrToID := make(map[string]uint32, len(o.nodes))
	// Also check for conflicts with existing nodes in the manager
	for _, existingNode := range mgr.Nodes() {
		addrToID[existingNode.Address()] = existingNode.ID()
	}
	// Sort IDs to ensure deterministic processing order
	nodes = make(Configuration, 0, len(o.nodes))
	for _, id := range slices.Sorted(maps.Keys(o.nodes)) {
		n := o.nodes[id]
		if id == 0 {
			return nil, fmt.Errorf("config: node 0 is reserved")
		}
		// Check if ID already exists in manager
		if existingNode, found := mgr.Node(id); found {
			// Only error if this is a new node with existing ID (not same node)
			if existingNode.Address() != n.Addr() {
				return nil, fmt.Errorf("config: node %d already in use", id)
			}
			// Same node, just add to configuration
			nodes = append(nodes, existingNode)
			continue
		}
		// Check for duplicate address
		if existingID, exists := addrToID[n.Addr()]; exists {
			return nil, fmt.Errorf("config: address %s already in use by node %d", n.Addr(), existingID)
		}
		addrToID[n.Addr()] = id
		node, err := mgr.newNode(n.Addr(), id)
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
	// Build a map of addresses to IDs to check for duplicates
	addrToID := make(map[string]uint32, len(o.addrsList))
	// Check for conflicts with existing nodes in the manager
	for _, existingNode := range mgr.Nodes() {
		addrToID[existingNode.Address()] = existingNode.ID()
	}
	// Find the highest ID currently in use; IDs start from max(nodeIDs) + 1
	// If manager is empty, maxID is 0, so new IDs start from 1
	maxID := uint32(0)
	if nodeIDs := mgr.NodeIDs(); len(nodeIDs) > 0 {
		maxID = slices.Max(nodeIDs)
	}
	nodes = make(Configuration, 0, len(o.addrsList))
	for i, naddr := range o.addrsList {
		// Check for duplicate address
		if existingID, exists := addrToID[naddr]; exists {
			return nil, fmt.Errorf("config: address %s already in use by node %d", naddr, existingID)
		}
		id := maxID + uint32(i) + 1
		addrToID[naddr] = id
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
