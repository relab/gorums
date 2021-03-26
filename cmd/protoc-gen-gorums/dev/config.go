package dev

import (
	"github.com/relab/gorums"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	gorums.Configuration
	qspec QuorumSpec
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (c *Configuration) Nodes() []*Node {
	nodes := make([]*Node, 0, c.Size())
	for _, n := range c.Configuration {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}

// With returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c Configuration) With(d *Configuration) gorums.NodeListOption {
	return c.Configuration.With(d.Configuration)
}

// Without returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c Configuration) Without(rm *Configuration) gorums.NodeListOption {
	return c.Configuration.Without(rm.Configuration)
}
