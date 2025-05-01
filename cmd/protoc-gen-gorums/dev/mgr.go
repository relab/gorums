package dev

import (
	"cmp"
	"fmt"

	"github.com/relab/gorums"
	"google.golang.org/grpc/encoding"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

// Manager maintains a connection pool of nodes on
// which quorum calls can be performed.
type Manager[idType cmp.Ordered] struct {
	*gorums.RawManager[idType]
}

// NewManager returns a new Manager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewManager[idType cmp.Ordered](opts ...gorums.ManagerOption) *Manager[idType] {
	return &Manager[idType]{
		RawManager: gorums.NewRawManager[idType](opts...),
	}
}

// NewConfiguration returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed. The QuorumSpec interface is also a ConfigOption.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (m *Manager[idType]) NewConfiguration(opts ...gorums.ConfigOption) (c *Configuration[idType], err error) {
	if len(opts) < 1 || len(opts) > 2 {
		return nil, fmt.Errorf("config: wrong number of options: %d", len(opts))
	}
	c = &Configuration[idType]{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case gorums.NodeListOption[idType]:
			c.RawConfiguration, err = gorums.NewRawConfiguration(m.RawManager, v)
			if err != nil {
				return nil, err
			}
		case QuorumSpec[idType]:
			// Must be last since v may match QuorumSpec if it is interface{}
			c.qspec = v
		default:
			return nil, fmt.Errorf("config: unknown option type: %v", v)
		}
	}
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec[idType]); !empty && c.qspec == nil {
		return nil, fmt.Errorf("config: missing required QuorumSpec")
	}
	// initialize the nodes slice
	c.nodes = make([]*Node[idType], c.Size())
	for i, n := range c.RawConfiguration {
		c.nodes[i] = &Node[idType]{n}
	}
	return c, nil
}

// Nodes returns a slice of available nodes on this manager.
// IDs are returned in the order they were added at creation of the manager.
func (m *Manager[idType]) Nodes() []*Node[idType] {
	gorumsNodes := m.RawManager.Nodes()
	nodes := make([]*Node[idType], len(gorumsNodes))
	for i, n := range gorumsNodes {
		nodes[i] = &Node[idType]{n}
	}
	return nodes
}
