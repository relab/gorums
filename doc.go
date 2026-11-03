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
//   - remotecall: a plain unary call to a single node. A method without a
//     call type option is a remote call.
//   - multicast: a one-way call to every node in a configuration.
//   - unicast: a one-way call to a single node.
//
// Quorum calls and one-way calls are dispatched lazily. The call returns a
// handle, and the request is sent only when the caller uses the handle.
// Lazy dispatch lets a call site attach client interceptors to the handle
// before the request is sent. A remote call sends its request at once.
//
// A quorum call sends its request on a terminal method, on Async, on
// Correctable, or on a range over Results. Async sends the request at once
// and leaves the replies to a later terminal method. Correctable returns
// progressively better results. A one-way call sends its request on Send or
// on Async.
//
// A call that fails returns its error from the method that collects the
// result. For a quorum call, that is a terminal method. For a one-way call,
// it is Send. If the caller uses Async, the returned handle collects the
// result later: the terminal methods of [Responses] or [OnewayAsync.Wait]
// return the error.
//
// # Service clients
//
// For each service S, protoc-gen-gorums generates up to two client
// interfaces. SClient has the quorum calls and multicasts, and
// NewSClient builds it from a [Config]. SNodeClient has the remote calls
// and unicasts, and NewSNodeClient builds it from a [Node]. The call type in
// the proto file decides which client a method belongs to. A service with
// only one kind of call gets only that client. Every call takes a
// [context.Context] and the request:
//
//	client := pb.NewStorageClient(cfg)
//	reply, err := client.ReadQC(ctx, req).Majority()
//
//	leader := pb.NewStorageNodeClient(cfg.Node(leaderID))
//	reply, err = leader.ReadRPC(ctx, req)
//
// A node client holds only its node, so building one allocates nothing.
// Build a node client where you need it, and do not keep it. If the node is
// nil, a remote call or unicast fails with [ErrNoNode]. On an empty
// configuration, a quorum call or multicast fails with [ErrEmptyConfig].
// To call another configuration, build a new SClient from it.
// A client does not own its configuration or node; the close function returned
// by [NewConfig] closes the connections.
//
// A Go package may hold several services, in one proto file or in several.
// Each service gets its own clients, server interface, and register function.
// The clients of all services may share one [Config], and one [Server] may
// register all services. The generated names of the services in one Go
// package must not collide; for example, services NewS and S both generate
// the name NewSClient, and protoc-gen-gorums reports such a collision.
//
// # Configurations and nodes
//
// A [Node] is a client-side handle to a peer reached through a network or
// local transport. A configuration is an immutable, ordered set of nodes built
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
