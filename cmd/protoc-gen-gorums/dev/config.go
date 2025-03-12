package dev

import (
	"fmt"

	"github.com/relab/gorums"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	gorums.RawConfiguration
	qspec QuorumSpec
	nodes []*Node
}

// NewConfiguration returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed.
// Nodes can be supplied using WithNodeMap or WithNodeList.
// Using any other type of NodeListOption will not work.
// The ManagerOption list controls how the nodes in the configuration are created.
func NewConfiguration(qspec QuorumSpec, cfg gorums.NodeListOption, opts ...gorums.ManagerOption) (c *Configuration, err error) {
	c = &Configuration{
		qspec: qspec,
	}
	c.RawConfiguration, err = gorums.NewRawConfiguration(cfg, opts...)
	if err != nil {
		return nil, err
	}
	c.nodes = make([]*Node, c.Size())
	for i, n := range c.RawConfiguration.RawNodes {
		c.nodes[i] = &Node{n}
	}
	return c, nil
}

// ConfigurationFromRaw returns a new configuration from the given raw configuration and QuorumSpec.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfiguration, qspec2)
func ConfigurationFromRaw(rawCfg gorums.RawConfiguration, qspec QuorumSpec) (*Configuration, error) {
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec); !empty && qspec == nil {
		return nil, fmt.Errorf("config: missing required QuorumSpec")
	}
	newCfg := &Configuration{
		RawConfiguration: rawCfg,
		qspec:            qspec,
	}
	// initialize the nodes slice
	newCfg.nodes = make([]*Node, newCfg.Size())
	for i, n := range rawCfg.Nodes() {
		newCfg.nodes[i] = &Node{n}
	}
	return newCfg, nil
}

// SubConfiguration allows for making a new Configuration from the ManagerOption list and
// node list of another configuration,
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (c *Configuration) SubConfiguration(qspec QuorumSpec, cfg gorums.NodeListOption) (subCfg *Configuration, err error) {
	subCfg = &Configuration{
		qspec: qspec,
	}
	subCfg.RawConfiguration, err = c.SubRawConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	subCfg.nodes = make([]*Node, subCfg.Size())
	for i, n := range subCfg.RawConfiguration.Nodes() {
		subCfg.nodes[i] = &Node{n}
	}
	return subCfg, nil
}

// Close closes a configuration created from the NewConfiguration method
//
// NOTE: A configuration created with ConfigurationFromRaw or SubConfiguration closes and
// is closed when the original configuration or any of the subconfigurations are closed.
// If you want the configurations to be independent you need to use NewConfiguration
func (c *Configuration) Close() error {
	return c.RawConfiguration.Close()
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
func (c *Configuration) Nodes() []*Node {
	return c.nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c Configuration) And(d *Configuration) gorums.NodeListOption {
	return c.RawConfiguration.And(d.RawConfiguration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c Configuration) Except(rm *Configuration) gorums.NodeListOption {
	return c.RawConfiguration.Except(rm.RawConfiguration)
}
