// Package impl implements the client-side call engine for Gorums.
//
// It holds the in-flight call handles and their aggregation logic: [Call] and
// [OnewayCall], the [CallContext] passed to interceptors, the [Responses]
// terminal methods, the [Correctable] future, and the client interceptor API
// ([ClientInterceptor], [MapRequest], [MapResponse]). It also provides the
// call constructors ([QuorumCall], [QuorumCallStream], [Multicast], [Unicast],
// [RemoteCall]) that generated code reaches through package gorumsimpl.
//
// The engine operates on the connectivity types in [conn] and sends through
// [conn.NodeTransport]. Package gorums re-exports the user-facing handle types
// here as aliases; application code must not import this package directly.
package impl
