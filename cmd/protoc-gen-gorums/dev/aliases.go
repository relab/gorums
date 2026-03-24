package dev

import gorums "github.com/relab/gorums"

// Type aliases for important Gorums types to make them more accessible
// from user code already interacting with the generated code.
type (
	Configuration = gorums.Configuration
	Node          = gorums.Node
	NodeContext   = gorums.NodeContext
	ConfigContext = gorums.ConfigContext
)

// Use the aliased types to add them to the reserved identifiers list.
// This prevents users from defining message types with these names.
var (
	_ = (*Configuration)(nil)
	_ = (*Node)(nil)
	_ = (*NodeContext)(nil)
	_ = (*ConfigContext)(nil)
)

// NewConfig returns a new [Configuration] based on the provided nodes and dial options.
//
// Example:
//
//	cfg, err := NewConfig(
//	    gorums.WithNodeList([]string{"localhost:8080", "localhost:8081", "localhost:8082"}),
//	    gorums.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
//	)
func NewConfig(nodes gorums.NodeListOption, opts ...gorums.DialOption) (Configuration, error) {
	return gorums.NewConfig(nodes, opts...)
}
