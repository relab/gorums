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
func WithNodes[T NodeAddress](nodes map[ID]T) NodeSource {
	return conn.WithNodes(nodes)
}

// WithNodeList returns a NodeSource for the provided list of node addresses.
// Unique Node IDs are generated sequentially starting from the maximum existing
// node ID plus one, or from 1 if no nodes exist, preventing conflicts with
// existing nodes.
func WithNodeList(addrsList []string) NodeSource {
	return conn.WithNodeList(addrsList)
}

// ID identifies a node. The application chooses the IDs of configured nodes
// with [WithNodes], or [WithNodeList] assigns them in list order. Servers
// configured with [WithPeers] must agree on the ID of each peer, because a
// server announces its own ID when it connects. ID 0 is reserved:
// it marks a handler-only server and a client that announces no ID. A server
// assigns IDs from 2^20 upward to back-channel clients, skipping configured
// IDs. Under [WithStreamDedup], the peer with the lower ID dials the other.
type ID = conn.ID

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

// NewConfig returns a new [Config] based on the provided nodes and dial
// options, and a function that closes the configuration's connection pool.
//
// The returned configuration and every configuration derived from it, for
// example with [Config.Extend], [Config.Remove], or [Config.Sort], share one
// connection pool. Only the returned close function closes that pool, and it
// closes every node in it, including nodes that [Config.Extend] added later.
// After the close function returns:
//   - calls to the pool's nodes fail with an Unavailable "node closed" error;
//   - [Config.Extend] on any configuration in the pool returns an error.
//
// One exception: a node that runs in-process, or that reuses a server's
// inbound stream, owns no connection, so calls to it still work.
//
// The close function is idempotent and safe for concurrent use; every call
// returns after the nodes are closed. It returns no error, because closing a
// connection has no failure that a caller can act on.
//
// On error, NewConfig returns a nil configuration and a nil close function.
//
// Example:
//
//	cfg, closeFn, err := NewConfig(
//	    gorums.WithNodeList([]string{"localhost:8080", "localhost:8081", "localhost:8082"}),
//	    gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
//	)
//	if err != nil {
//	    return err
//	}
//	defer closeFn()
func NewConfig(nodes NodeSource, opts ...DialOption) (Config, func(), error) {
	return conn.NewConfig(nodes, opts...)
}
