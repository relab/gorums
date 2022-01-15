package dev

import (
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
type Manager struct {
	*gorums.RawManager
}

// NewManager returns a new Manager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewManager(opts ...gorums.ManagerOption) (mgr *Manager) {
	mgr = &Manager{}
	mgr.RawManager = gorums.NewRawManager(opts...)
	return mgr
}

// NewConfiguration returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed. The QuorumSpec interface is also a ConfigOption.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (m *Manager) NewConfiguration(opts ...gorums.ConfigOption) (c Configuration, err error) {
	if len(opts) < 1 || len(opts) > 2 {
		return c, gorums.ConfigCreationError(fmt.Errorf("wrong number of options: %d", len(opts)))
	}
	var (
		qspec QuorumSpec
		nodes gorums.NodeListOption
	)
	for _, opt := range opts {
		switch v := opt.(type) {
		case gorums.NodeListOption:
			nodes = v
		case QuorumSpec:
			// Must be last since v may match QuorumSpec if it is interface{}
			qspec = v
		default:
			return c, gorums.ConfigCreationError(fmt.Errorf("unknown option type: %v", v))
		}
	}
	rawCfg, err := gorums.NewRawConfiguration[Node](m.RawManager, qspec, nodes)
	if err != nil {
		return c, gorums.ConfigCreationError(err)
	}
	return ConfigurationFromRaw(rawCfg, qspec), nil
}

// Nodes returns a slice of available nodes on this manager.
// IDs are returned in the order they were added at creation of the manager.
func (m *Manager) Nodes() []*Node {
	gorumsNodes := m.RawManager.Nodes()
	nodes := make([]*Node, 0, len(gorumsNodes))
	for _, n := range gorumsNodes {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}
