package dev

import (
	"github.com/relab/gorums"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	gorums.RawConfiguration
}

// NewConfiguration returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed.
// Nodes can be supplied using WithNodeMap or WithNodeList.
// Using any other type of NodeListOption will not work.
// The ManagerOption list controls how the nodes in the configuration are created.
func NewConfiguration(cfg gorums.NodeListOption, opts ...gorums.ManagerOption) (c *Configuration, err error) {
	c = &Configuration{}
	c.RawConfiguration, err = gorums.NewRawConfiguration(cfg, opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ConfigurationFromRaw returns a new configuration from the given raw configuration.
//
//	cfg1, err := pb.NewConfiguration(nodeList, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig)
func ConfigurationFromRaw(rawCfg gorums.RawConfiguration) (*Configuration, error) {
	newCfg := &Configuration{
		RawConfiguration: rawCfg,
	}
	return newCfg, nil
}

// SubConfiguration allows for making a new Configuration from the ManagerOption list and
// node list of another configuration,
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (c *Configuration) SubConfiguration(cfg gorums.NodeListOption) (subCfg *Configuration, err error) {
	subCfg = &Configuration{}
	subCfg.RawConfiguration, err = c.SubRawConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	return subCfg, nil
}

// Close closes the nodes which aren't used by other subconfigurations
func (c *Configuration) Close() error {
	return c.RawConfiguration.Close()
}

// Nodes returns a slice of the configuration nodes. Sorted by node id.
//
// NOTE: mutating the returned slice is not supported.
func (c *Configuration) Nodes() []*Node {
	nodes := make([]*Node, c.Size())
	for i, n := range c.RawConfiguration.RawNodes {
		nodes[i] = &Node{n}
	}
	return nodes
}

// AllNodes returns a slice of each available node of all subconfigurations. Sorted by node id.
//
// NOTE: mutating the returned slice is not supported.
func (c *Configuration) AllNodes() []*Node {
	rawNodes := c.RawConfiguration.AllNodes()
	nodes := make([]*Node, len(rawNodes))
	for i, n := range rawNodes {
		nodes[i] = &Node{n}
	}
	return nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c *Configuration) And(d *Configuration) gorums.NodeListOption {
	return c.RawConfiguration.And(d.RawConfiguration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c *Configuration) Except(rm *Configuration) gorums.NodeListOption {
	return c.RawConfiguration.Except(rm.RawConfiguration)
}
