package gorums

import (
	"fmt"
)

// RawConfiguration represents a static set of nodes on which quorum calls may be invoked.
//
// This type is intended to be used by generated code.
// You should use the generated `Configuration` type instead.
type RawConfiguration []*RawNode

// NewRawConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewRawConfiguration(mgr *RawManager, opt NodeListOption) (nodes RawConfiguration, err error) {
	if opt == nil {
		return nil, ConfigCreationError(fmt.Errorf("missing required node list"))
	}
	return opt.newConfig(mgr)
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c RawConfiguration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
func (c RawConfiguration) Nodes() []*RawNode {
	return c
}

// Size returns the number of nodes in this configuration.
func (c RawConfiguration) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c RawConfiguration) Equal(b RawConfiguration) bool {
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

func (c RawConfiguration) getMsgID() uint64 {
	return c[0].mgr.getMsgID()
}
