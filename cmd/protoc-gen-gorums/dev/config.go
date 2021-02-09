package dev

import (
	"fmt"

	"github.com/relab/gorums"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	*gorums.Configuration
	mgr   *Manager
	qspec QuorumSpec
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (c *Configuration) Nodes() []*Node {
	gorumsNodes := c.Configuration.Nodes()
	nodes := make([]*Node, 0, len(gorumsNodes))
	for _, n := range gorumsNodes {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}

// NewConfig returns a configuration for the given node addresses and quorum spec.
// The returned func() must be called to close the underlying connections.
// This is an experimental API.
func NewConfig(qspec QuorumSpec, opts ...gorums.ManagerOption) (*Configuration, func(), error) {
	man, err := NewManager(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create manager: %v", err)
	}
	c, err := man.NewConfiguration(man.NodeIDs(), qspec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create configuration: %v", err)
	}
	return c, func() { man.Close() }, nil
}
