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
type RawConfiguration struct {
	RawNodes []*RawNode
	*RawManager
}

// NewRawConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList.
func NewRawConfiguration(cfg NodeListOption, opts ...ManagerOption) (nodes RawConfiguration, err error) {
	if cfg == nil {
		return RawConfiguration{}, fmt.Errorf("config: missing required node list")
	}
	mgr := NewRawManager(opts...)
	return cfg.newConfig(mgr)
}

// SubRawConfiguration returns a configuration from another configuration and a list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (c *RawConfiguration) SubRawConfiguration(cfg NodeListOption) (nodes RawConfiguration, err error) {
	if cfg == nil {
		return RawConfiguration{}, fmt.Errorf("config: missing required node list")
	}
	return cfg.newConfig(c.RawManager)
}

// Close closes a raw configuration created from the NewRawConfiguration method
func (c *RawConfiguration) Close() error {
	c.RawManager.Close()
	return nil
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c RawConfiguration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c.RawNodes))
	for i, node := range c.RawNodes {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c RawConfiguration) Nodes() []*RawNode {
	return c.RawNodes
}

// Size returns the number of nodes in this configuration.
func (c RawConfiguration) Size() int {
	return len(c.RawNodes)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c RawConfiguration) Equal(b RawConfiguration) bool {
	if len(c.RawNodes) != len(b.RawNodes) {
		return false
	}
	for i := range c.RawNodes {
		if c.RawNodes[i].ID() != b.RawNodes[i].ID() {
			return false
		}
	}
	return true
}

func (c RawConfiguration) getMsgID() uint64 {
	return c.RawNodes[0].mgr.getMsgID()
}
