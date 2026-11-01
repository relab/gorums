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
// Quorum calls and one-way calls return handles that dispatch when consumed.
// Remote calls execute immediately.
// Lazy dispatch is what lets a call site attach client interceptors before the
// request is sent. [Call.Async] sends the request at once and leaves the
// replies to a later terminal method, and [Responses.Correctable] reports
// progressively better results.
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
// A node client holds only its node, which may be outside cfg.
// If the node is nil, remote calls return [ErrNoNode] and unicast handles
// report it when consumed. A quorum call or multicast on an empty
// configuration sends nothing and reports [ErrEmptyConfig] when consumed.
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
