package gorums

import (
	"fmt"
)

// Configuration represents a static set of nodes on which quorum calls may be invoked.
type Configuration []*Node

// NewConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewConfiguration(mgr *Manager, opt NodeListOption) (nodes Configuration, err error) {
	if opt == nil {
		return nil, ConfigCreationError(fmt.Errorf("missing required node list"))
	}
	return opt.newConfig(mgr)
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

// Contains returns true if nodeID is in configuration c.
func (c Configuration) Contains(nodeID uint32) bool {
	for i := range c {
		if c[i].ID() == nodeID {
			return true
		}
	}
	return false
}

func (c Configuration) getMsgID() uint64 {
	return c[0].mgr.getMsgID()
}
