package dev

import (
	"fmt"
	"time"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	id      int
	gid     uint32
	nodes   []int
	mgr     *Manager
	quorum  int
	timeout time.Duration
}

// ID reports the local identifier for the configuration.
func (c *Configuration) ID() int {
	return c.id
}

// GlobalID reports the unique global identifier for the configuration.
func (c *Configuration) GlobalID() uint32 {
	return c.gid
}

// Nodes returns a slice containing the local ids of all the nodes in the
// configuration.
func (c *Configuration) Nodes() []int { return c.nodes }

// Quorum returns the quourm size for the configuration.
func (c *Configuration) Quorum() int {
	return c.quorum
}

// Size returns the number of nodes in the configuration.
func (c *Configuration) Size() int {
	return len(c.nodes)
}

func (c *Configuration) String() string {
	return fmt.Sprintf("configuration %d | gid: %d", c.id, c.gid)
}

// Equal returns a boolean reporting whether a and b represents the same
// configuration.
func Equal(a, b *Configuration) bool { return a.gid == b.gid }

// NewTestConfiguration returns a new configuration with quorum size q and
// node size n. No other fields are set. Configurations returned from this
// constructor should only be used when testing quorum functions.
func NewTestConfiguration(q, n int) *Configuration {
	return &Configuration{
		quorum: q,
		nodes:  make([]int, n),
	}
}
