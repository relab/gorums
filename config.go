package gorums

import (
	"fmt"
)

// RawConfiguration represents a static set of nodes on which quorum calls may be invoked.
//
// NOTE: mutating the configuration is not supported.
//
// This type is intended to be used by generated code.
// You should use the generated `Configuration` type instead.
type RawConfiguration[NODE AsRawNode] []NODE

// NewRawConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewRawConfiguration[NODE AsRawNode](mgr *RawManager, opt NodeListOption[NODE]) (nodes RawConfiguration[NODE], err error) {
	if opt == nil {
		return nil, ConfigCreationError(fmt.Errorf("missing required node list"))
	}
	return opt.newConfig(mgr)
}

func NewRawConfigurationFromNodeSlice[NODE AsRawNode](nodes []NODE) RawConfiguration[NODE] {
	return nodes
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c RawConfiguration[NODE]) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.AsRaw().ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c RawConfiguration[NODES]) Nodes() []NODES {
	return c
}

// Size returns the number of nodes in this configuration.
func (c RawConfiguration[NODES]) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c RawConfiguration[NODES]) Equal(b RawConfiguration[NODES]) bool {
	if len(c) != len(b) {
		return false
	}
	for i := range c {
		if c[i].AsRaw().ID() != b[i].AsRaw().ID() {
			return false
		}
	}
	return true
}

// shortcut to the manager through one of the nodes
func (c RawConfiguration[NODES]) getMsgID() uint64 {
	return c[0].AsRaw().mgr.getMsgID()
}
