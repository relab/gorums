package dev

import (
	"cmp"
	"fmt"

	"github.com/relab/gorums"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration[idType cmp.Ordered] struct {
	gorums.RawConfiguration[idType]
	qspec QuorumSpec[idType]
	nodes []*Node[idType]
}

// ConfigurationFromRaw returns a new Configuration from the given raw configuration and QuorumSpec.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func ConfigurationFromRaw[idType cmp.Ordered](rawCfg gorums.RawConfiguration[idType], qspec QuorumSpec[idType]) (*Configuration[idType], error) {
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec[idType]); !empty && qspec == nil {
		return nil, fmt.Errorf("config: missing required QuorumSpec")
	}
	newCfg := &Configuration[idType]{
		RawConfiguration: rawCfg,
		qspec:            qspec,
	}
	// initialize the nodes slice
	newCfg.nodes = make([]*Node[idType], newCfg.Size())
	for i, n := range rawCfg {
		newCfg.nodes[i] = &Node[idType]{n}
	}
	return newCfg, nil
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
func (c *Configuration[idType]) Nodes() []*Node[idType] {
	return c.nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c Configuration[idType]) And(d *Configuration[idType]) gorums.NodeListOption[idType] {
	return c.RawConfiguration.And(d.RawConfiguration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c Configuration[idType]) Except(rm *Configuration[idType]) gorums.NodeListOption[idType] {
	return c.RawConfiguration.Except(rm.RawConfiguration)
}
