package gorums

import (
	"fmt"
)

// Configuration represents a static set of nodes on which quorum calls may be invoked.
//
// NOTE: mutating the configuration is not supported.
//
// This type is intended to be used by generated code.
// You should use the generated `Configuration` type instead.
type Configuration struct {
	nodes []*Node
	mgr   *manager
}

// NewConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList.
func NewConfiguration(nodes NodeListOption, opts ...ManagerOption) (cfg Configuration, err error) {
	if nodes == nil {
		return Configuration{}, fmt.Errorf("config: missing required node list")
	}
	mgr := newManager(opts...)

	cfg, err = nodes.newConfig(mgr)
	for _, n := range cfg.nodes {
		n.obtain()
	}

	return cfg, err
}

// SubConfiguration returns a configuration from another configuration and a list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (c *Configuration) SubConfiguration(nodes NodeListOption) (cfg Configuration, err error) {
	if nodes == nil {
		return Configuration{}, fmt.Errorf("config: missing required node list")
	}

	cfg, err = nodes.newConfig(c.mgr)
	for _, n := range cfg.nodes {
		n.obtain()
	}

	return cfg, err
}

// CloseAll closes the configurations and all of its subconfigurations
func (c *Configuration) CloseAll() error {
	c.mgr.close()
	return nil
}

// Close closes the nodes which are not part of another subconfiguration
// it is only meant to be used once
func (c *Configuration) Close() error {
	for _, n := range c.nodes {
		err := n.release()
		if err != nil {
			return err
		}
	}
	*c = Configuration{}
	return nil
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c.nodes))
	for i, node := range c.nodes {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c Configuration) Nodes() []*Node {
	return c.nodes
}

// AllNodes returns the nodes in this configuration and all subconfigurations.
func (c Configuration) AllNodeIDs() []uint32 {
	return c.mgr.nodeIDs()
}

// AllNodes returns the nodes in this configuration and all subconfigurations.
func (c Configuration) AllNodes() []*Node {
	return c.mgr.getNodes()
}

// Size returns the number of nodes in this configuration.
func (c Configuration) Size() int {
	return len(c.nodes)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c Configuration) Equal(b Configuration) bool {
	if len(c.nodes) != len(b.nodes) {
		return false
	}
	for i := range c.nodes {
		if c.nodes[i].ID() != b.nodes[i].ID() {
			return false
		}
	}
	return true
}

func (c Configuration) getMsgID() uint64 {
	return c.nodes[0].mgr.getMsgID()
}
