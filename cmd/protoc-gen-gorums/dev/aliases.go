package dev

import gorums "github.com/relab/gorums"

// Type aliases for important Gorums types to make them more accessible
// from user code already interacting with the generated code.
type (
	Configuration = gorums.Configuration
	Manager       = gorums.Manager
	Node          = gorums.Node
)

// NewManager returns a new Manager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewManager(opts ...gorums.ManagerOption) *Manager {
	return gorums.NewManager(opts...)
}

// NewConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewConfiguration(mgr *Manager, opt gorums.NodeListOption) (Configuration, error) {
	return gorums.NewConfiguration(mgr, opt)
}

// NewNode returns a new node for the provided address.
func NewNode(addr string) (*Node, error) {
	return gorums.NewNode(addr)
}

// Use the aliased types to add them to the reserved identifiers list.
// This prevents users from defining message types with these names.
var (
	_ = (*Configuration)(nil)
	_ = (*Manager)(nil)
	_ = (*Node)(nil)
)
