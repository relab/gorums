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
type RawConfiguration[NODE RawNodeConstraint, QSPEC any] struct {
	nodes []NODE
	qspec QSPEC
}

// NewRawConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewRawConfiguration[NODE RawNodeConstraint, QSPEC any](mgr *RawManager, qspec QSPEC, opt NodeListOption) (cfg RawConfiguration[NODE, QSPEC], err error) {
	if opt == nil {
		return cfg, ConfigCreationError(fmt.Errorf("missing required node list"))
	}
	rawNodes, err := opt.newConfig(mgr)
	if err != nil {
		return cfg, ConfigCreationError(err)
	}
	genNodes := make([]NODE, 0, len(rawNodes))
	for _, n := range rawNodes {
		genNodes = append(genNodes, NODE{n})
	}
	return NewRawConfigurationFromNodeSlice(genNodes, qspec), nil
}

func NewRawConfigurationFromNodeSlice[NODE RawNodeConstraint, QSPEC any](nodes []NODE, qspec QSPEC) RawConfiguration[NODE, QSPEC] {
	return RawConfiguration[NODE, QSPEC]{nodes, qspec}
}

// NodeIDs returns a slice of the IDs of the nodes in this configuration.
func (c RawConfiguration[NODE, QSPEC]) NodeIDs() []uint32 {
	ids := make([]uint32, len(c.nodes))
	for i, node := range c.nodes {
		ids[i] = node.AsRaw().ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c RawConfiguration[NODES, QSPEC]) Nodes() []NODES {
	return c.nodes
}

// QSpec returns the quorum specification object.
func (c RawConfiguration[NODES, QSPEC]) QSpec() QSPEC {
	return c.qspec
}

// Size returns the number of nodes in this configuration.
func (c RawConfiguration[NODES, QSPEC]) Size() int {
	return len(c.nodes)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c RawConfiguration[NODES, QSPEC]) Equal(b RawConfiguration[NODES, QSPEC]) bool {
	if len(c.nodes) != len(b.nodes) {
		return false
	}
	for i := range c.nodes {
		if c.nodes[i].AsRaw().ID() != b.nodes[i].AsRaw().ID() {
			return false
		}
	}
	return true
}

// shortcut to the manager through one of the nodes
func (c RawConfiguration[NODES, QSPEC]) getMsgID() uint64 {
	return c.nodes[0].AsRaw().mgr.getMsgID()
}

func (c RawConfiguration[NODES, QSPEC]) rawNodes() (nodes []*RawNode) {
	nodes = make([]*RawNode, 0, len(c.nodes))
	for _, n := range c.nodes {
		nodes = append(nodes, n.AsRaw())
	}
	return nodes
}
