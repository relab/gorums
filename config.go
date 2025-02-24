package gorums

import (
	"cmp"
	"fmt"
)

// RawConfiguration represents a static set of nodes on which quorum calls may be invoked.
//
// NOTE: mutating the configuration is not supported.
//
// This type is intended to be used by generated code.
// You should use the generated `Configuration` type instead.
type RawConfiguration[idType cmp.Ordered] []*RawNode[idType]

// NewRawConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewRawConfiguration[idType cmp.Ordered](mgr *RawManager[idType], opt NodeListOption[idType]) (nodes RawConfiguration[idType], err error) {
	if opt == nil {
		return nil, fmt.Errorf("config: missing required node list")
	}
	return opt.newConfig(mgr)
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c RawConfiguration[idType]) NodeIDs() []idType {
	ids := make([]idType, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c RawConfiguration[idType]) Nodes() []*RawNode[idType] {
	return c
}

// Size returns the number of nodes in this configuration.
func (c RawConfiguration[idType]) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c RawConfiguration[idType]) Equal(b RawConfiguration[idType]) bool {
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

func (c RawConfiguration[idType]) getMsgID() uint64 {
	return c[0].mgr.getMsgID()
}
