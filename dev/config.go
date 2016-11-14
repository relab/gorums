package dev

import "fmt"

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	id    uint32
	nodes []*Node
	n     int
	mgr   *Manager
	qspec QuorumSpec
}

// ID reports the identifier for the configuration.
func (c *Configuration) ID() uint32 {
	return c.id
}

// NodeIDs returns a slice containing the local ids of all the nodes in the
// configuration.
func (c *Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c.nodes))
	for i, node := range c.nodes {
		ids[i] = node.ID()
	}
	return ids
}

// Size returns the number of nodes in the configuration.
func (c *Configuration) Size() int {
	return c.n
}

func (c *Configuration) String() string {
	return fmt.Sprintf("configuration %d", c.id)
}

func (c *Configuration) tstring() string {
	return fmt.Sprintf("config-%d", c.id)
}

// Equal returns a boolean reporting whether a and b represents the same
// configuration.
func Equal(a, b *Configuration) bool { return a.id == b.id }

// NewTestConfiguration returns a new configuration with quorum size q and
// node size n. No other fields are set. Configurations returned from this
// constructor should only be used when testing quorum functions.
func NewTestConfiguration(q, n int) *Configuration {
	return &Configuration{
		nodes: make([]*Node, n),
	}
}
