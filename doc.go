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
//     return a handle whose terminal methods (First, Majority, All, Threshold,
//     Aggregate, Reduce, Matching) wait for the responses and aggregate them
//     into a result.
//   - rpc: a plain unary call to a single node.
//   - multicast: a one-way call to every node in a configuration.
//   - unicast: a one-way call to a single node.
//
// Calls are dispatched lazily: the request is sent on the first consuming
// operation, such as a terminal method, [Call.Async], or a range over the
// response sequence.
// Lazy dispatch is what lets a call site attach client interceptors before the
// request is sent. [Call.Async] sends the request at once and leaves the
// replies to a later terminal method, and [Responses.Correctable] reports
// progressively better results.
//
// # Service clients
//
// For each service S, protoc-gen-gorums generates one client type, SClient,
// bound to a [Config]. The client type is a namespace for the service: it
// keeps the service's calls together and avoids package-level name conflicts.
// The quorum call and multicast methods take a [context.Context] and the
// request, and execute on the client's configuration. The remote call and
// unicast methods also take the target [Node] after the context, and execute
// on that node:
//
//	client := pb.NewStorageClient(cfg)
//	reply, err := client.ReadQC(ctx, req).Majority()
//	reply, err = client.ReadRPC(ctx, node, req)
//
// A node call does not use the client's configuration, so the zero SClient is
// ready for node calls:
//
//	reply, err = pb.StorageClient{}.ReadRPC(ctx, node, req)
//
// A client holds only its configuration, so creating one allocates nothing. A
// client does not own its configuration; the close function returned by
// [NewConfig] closes the connections.
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
