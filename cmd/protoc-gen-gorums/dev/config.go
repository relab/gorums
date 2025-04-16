package dev

import (
	"github.com/relab/gorums"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	gorums.RawConfiguration
	nodes []*Node
}

// ConfigurationFromRaw returns a new Configuration from the given raw configuration and QuorumSpec.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func ConfigurationFromRaw(rawCfg gorums.RawConfiguration) (*Configuration, error) {
	newCfg := &Configuration{
		RawConfiguration: rawCfg,
	}
	// initialize the nodes slice
	newCfg.nodes = make([]*Node, newCfg.Size())
	for i, n := range rawCfg {
		newCfg.nodes[i] = &Node{n}
	}
	return newCfg, nil
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
