package gorums

import (
	"context"
	"fmt"
)

// ConfigContext is a context that carries a configuration for quorum calls.
// It embeds context.Context and provides access to the RawConfiguration.
//
// Use [WithConfigContext] to create a ConfigContext from an existing context.
type ConfigContext struct {
	context.Context
	cfg RawConfiguration
}

// WithConfigContext creates a new ConfigContext from the given parent context
// and configuration. The configuration must not be empty.
//
// Example:
//
//	cfg, _ := gorums.NewRawConfiguration(mgr, gorums.WithNodeList(addrs))
//	ctx := gorums.WithConfigContext(context.Background(), cfg)
//	resp, err := paxos.Prepare(ctx, req)
func WithConfigContext(parent context.Context, cfg RawConfiguration) *ConfigContext {
	if len(cfg) == 0 {
		panic("gorums: WithConfigContext called with empty configuration")
	}
	return &ConfigContext{Context: parent, cfg: cfg}
}

// Configuration returns the RawConfiguration associated with this context.
func (c *ConfigContext) Configuration() RawConfiguration {
	return c.cfg
}

// RawConfiguration represents a static set of nodes on which quorum calls may be invoked.
//
// NOTE: mutating the configuration is not supported.
//
// This type is intended to be used by generated code.
// You should use the generated `Configuration` type instead.
type RawConfiguration []*RawNode

// NewRawConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewRawConfiguration(mgr *RawManager, opt NodeListOption) (nodes RawConfiguration, err error) {
	if opt == nil {
		return nil, fmt.Errorf("config: missing required node list")
	}
	return opt.newConfig(mgr)
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c RawConfiguration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c RawConfiguration) Nodes() []*RawNode {
	return c
}

// Size returns the number of nodes in this configuration.
func (c RawConfiguration) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c RawConfiguration) Equal(b RawConfiguration) bool {
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

func (c RawConfiguration) getMsgID() uint64 {
	return c[0].mgr.getMsgID()
}
