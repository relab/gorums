package gorums

import "github.com/relab/gorums/internal/conn"

// NodeSource identifies the set of nodes to build a [Config] from. Create one
// with [WithNodes] or [WithNodeList]; the interface is sealed so it can only
// be implemented within the gorums module.
type NodeSource = conn.NodeSource

// NodeAddress must be implemented by types that can be used as node addresses.
type NodeAddress = conn.NodeAddress

// WithNodes returns a NodeSource containing the provided mapping from
// application-specific IDs to types implementing NodeAddress.
// Node IDs must be greater than 0.
func WithNodes[T NodeAddress](nodes map[uint32]T) NodeSource {
	return conn.WithNodes(nodes)
}

// WithNodeList returns a NodeSource for the provided list of node addresses.
// Unique Node IDs are generated sequentially starting from the maximum existing
// node ID plus one, or from 1 if no nodes exist, preventing conflicts with
// existing nodes.
func WithNodeList(addrsList []string) NodeSource {
	return conn.WithNodeList(addrsList)
}

// Node encapsulates the state of a node on which a remote procedure call can be
// performed. Nodes are created as part of a [Config] built with [NewConfig].
type Node = conn.Node

// NodeContext is a context that carries a node for unicast and RPC calls.
// It embeds context.Context and provides access to the Node.
//
// Use [Node.Context] to create a NodeContext from an existing context.
type NodeContext = conn.NodeContext

// ByID compares nodes by their identifier in increasing order.
// It is compatible with [slices.SortFunc] and [Config.Sort].
var ByID = conn.ByID

// ByLastError compares nodes by their LastErr status.
// Nodes with no error sort before nodes with an error.
// It is compatible with [slices.SortFunc] and [Config.Sort].
var ByLastError = conn.ByLastError

// ByLatency compares nodes by their current latency estimate in ascending order.
// Nodes with no measurement yet (negative latency value) sort after nodes with a
// measurement. It is compatible with [slices.SortFunc] and [Config.Sort].
var ByLatency = conn.ByLatency

// Config represents a static set of nodes on which multicast or
// quorum calls may be invoked. A configuration is created using [NewConfig].
// A configuration should be treated as immutable. Therefore, methods that
// operate on a configuration always return a new Config instance.
type Config = conn.Config

// ConfigContext is a context that carries a configuration for multicast or
// quorum calls. It embeds context.Context and provides access to the configuration.
//
// Use [Config.Context] to create a ConfigContext from an existing context.
type ConfigContext = conn.ConfigContext

// NewConfig returns a new [Config] based on the provided nodes and dial options.
//
// Example:
//
//	cfg, err := NewConfig(
//	    gorums.WithNodeList([]string{"localhost:8080", "localhost:8081", "localhost:8082"}),
//	    gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
//	)
func NewConfig(nodes NodeSource, opts ...DialOption) (Config, error) {
	return conn.NewConfig(nodes, opts...)
}
