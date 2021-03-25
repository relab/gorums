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

// Add returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c Configuration) Add(d *Configuration) gorums.NodeListOption {
	return c.Configuration.Add(d.Configuration)
}

// Remove returns a NodeListOption that can be used to create a new configuration
// from configuration c without the nodes in configuration rm.
func (c Configuration) Remove(rm *Configuration) gorums.NodeListOption {
	return c.Configuration.Remove(rm.Configuration)
}
