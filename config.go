package gorums

import (
	"context"
	"fmt"
)

// ConfigContext is a context that carries a configuration for quorum calls.
// It embeds context.Context and provides access to the Configuration.
//
// Use [WithConfigContext] to create a ConfigContext from an existing context.
type ConfigContext struct {
	context.Context
	cfg Configuration
}

// WithConfigContext creates a new ConfigContext from the given parent context
// and configuration. The configuration must not be empty.
//
// Example:
//
//	cfg, _ := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
//	ctx := gorums.WithConfigContext(context.Background(), cfg)
//	resp, err := paxos.Prepare(ctx, req)
func WithConfigContext(parent context.Context, cfg Configuration) *ConfigContext {
	if len(cfg) == 0 {
		panic("gorums: WithConfigContext called with empty configuration")
	}
	return &ConfigContext{Context: parent, cfg: cfg}
}

// Configuration returns the Configuration associated with this context.
func (c ConfigContext) Configuration() Configuration {
	return c.cfg
}

// Configuration represents a static set of nodes on which quorum calls may be invoked.
//
// Mutating the configuration is not supported; instead, use NewConfiguration to create
// a new configuration.
type Configuration []*Node

// NewConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewConfiguration(mgr *Manager, opt NodeListOption) (nodes Configuration, err error) {
	if opt == nil {
		return nil, fmt.Errorf("config: missing required node list")
	}
	return opt.newConfig(mgr)
}

// NewConfig returns a new [Configuration] based on the provided [gorums.Option]s.
// It accepts exactly one [gorums.NodeListOption] and multiple [gorums.ManagerOption]s.
// You may use this function to create the initial configuration for a new manager.
//
// Example:
//
//		cfg, err := NewConfig(
//		    gorums.WithNodeList([]string{"localhost:8080", "localhost:8081", "localhost:8082"}),
//	        gorums.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
//		)
//
// This is a convenience function for creating a configuration without explicitly
// creating a manager first. However, the manager can be accessed using the
// [Configuration.Manager] method. This method should only be used once since it
// creates a new manager; if a manager already exists, use [NewConfiguration]
// instead, and provide the existing manager as the first argument.
func NewConfig(opts ...Option) (Configuration, error) {
	var (
		managerOptions []ManagerOption
		nodeListOption NodeListOption
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case ManagerOption:
			managerOptions = append(managerOptions, o)
		case NodeListOption:
			if nodeListOption != nil {
				return nil, fmt.Errorf("gorums: multiple NodeListOptions provided")
			}
			nodeListOption = o
		default:
			return nil, fmt.Errorf("gorums: unknown option type: %T", opt)
		}
	}
	if nodeListOption == nil {
		return nil, fmt.Errorf("gorums: missing required NodeListOption")
	}
	mgr := NewManager(managerOptions...)
	return NewConfiguration(mgr, nodeListOption)
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
func (c Configuration) Nodes() []*Node {
	return c
}

// Size returns the number of nodes in this configuration.
func (c Configuration) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c Configuration) Equal(b Configuration) bool {
	if len(c) != len(b) {
		return false
	}
	for i := range c {
		if c[i].ID() != b[i].ID() {
			return false
		}
	}
	return true
}

// Manager returns the Manager that manages this configuration's nodes.
// Returns nil if the configuration is empty.
func (c Configuration) Manager() *Manager {
	if len(c) == 0 {
		return nil
	}
	return c[0].mgr
}

// nextMsgID returns the next message ID from this client's manager.
func (c Configuration) nextMsgID() uint64 {
	return c[0].mgr.getMsgID()
}
