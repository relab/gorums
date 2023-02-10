package dev

import (
	"github.com/relab/gorums"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration gorums.RawConfiguration[Node, QuorumSpec]

// AsRaw returns the RawConfiguration.
func (c Configuration) AsRaw() gorums.RawConfiguration[Node, QuorumSpec] {
	return gorums.RawConfiguration[Node, QuorumSpec](c)
}

// ConfigurationFromRaw returns a new Configuration from the given raw configuration and QuorumSpec.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//  cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//  cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func ConfigurationFromRaw[NODE gorums.RawNodeConstraint, QSPEC QuorumSpec](rawCfg gorums.RawConfiguration[NODE, QSPEC], qspec QuorumSpec) Configuration {
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec); !empty && qspec == nil {
		panic("QuorumSpec may not be nil")
	}

	otherNodes := rawCfg.Nodes()
	ourNodes := make([]Node, rawCfg.Size())

	for i, node := range otherNodes {
		ourNodes[i] = Node{node.AsRaw()}
	}

	return Configuration(gorums.NewRawConfigurationFromNodeSlice(ourNodes, qspec))
}

// NodeIDs returns a slice of the IDs of the nodes in this configuration.
func (c Configuration) NodeIDs() []uint32 {
	return c.AsRaw().NodeIDs()
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c Configuration) Nodes() []Node {
	return c.AsRaw().Nodes()
}

// QSpec returns the quorum specification object.
func (c Configuration) QSpec() QuorumSpec {
	return c.AsRaw().QSpec()
}

// Size returns the number of nodes in this configuration.
func (c Configuration) Size() int {
	return c.AsRaw().Size()
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c Configuration) Equal(b Configuration) bool {
	return c.AsRaw().Equal(b.AsRaw())
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c Configuration) And(d *Configuration) gorums.NodeListOption {
	return c.AsRaw().And(d.AsRaw())
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c Configuration) Except(rm *Configuration) gorums.NodeListOption {
	return c.AsRaw().Except(rm.AsRaw())
}
