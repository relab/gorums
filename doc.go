// Package gorums is a runtime for quorum-based remote procedure calls over gRPC.
//
// Gorums lets a client invoke a method on a configuration of server nodes and
// aggregate their replies into a single result. It pairs a protoc plugin
// (protoc-gen-gorums) that turns annotated gRPC service definitions into thin,
// typed call wrappers with this package, the runtime that performs the
// fan-out, collects responses, and evaluates the quorum.
//
// # Call types
//
// A method's behavior is selected by one of four options in the .proto file:
//
//   - quorumcall: fan the request out to every node in a configuration and
//     return a handle whose terminal methods (First, Majority, All, Threshold)
//     block until the required number of successful responses arrive.
//   - rpc: a plain unary call to a single node.
//   - multicast: a one-way call to every node in a configuration.
//   - unicast: a one-way call to a single node.
//
// Calls are dispatched lazily: the request is sent on the first consuming
// operation, such as a terminal method or a range over the response sequence.
// Lazy dispatch is what lets a call site attach client interceptors before the
// request is sent and choose synchronous, asynchronous, or correctable aggregation
// at the call site.
//
// # Configurations and nodes
//
// A [Node] is a client-side handle to a remote (or co-located) peer, not the
// local process. A configuration is an immutable, ordered set of nodes built
// with [NewConfig] from a node source, e.g., [WithNodes] or [WithNodeList].
// Set operations such as [Config.Add], [Config.Remove], and [Config.Union]
// derive new configurations that share the underlying node connections, so
// protocol code can cheaply target subsets of the membership.
//
// # Servers
//
// A server hosts the generated service handlers and is created with
// [NewServer]. When a process is both a client and a server, a single
// bidirectional stream can carry calls in both directions.
package gorums

//go:generate protoc --go_out=paths=source_relative:. gorums.proto
