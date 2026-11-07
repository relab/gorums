# Migration Guide: Gorums v0.11 to v0.13

This guide covers the v0.12 API redesign and the generated service clients in v0.13.
The examples use a `Storage` service in package `pb` with quorum call `ReadQC`, remote call `ReadRPC`, and multicast `WriteMulticast`.
Replace these names with those of your service.
For the earlier QuorumSpec-to-iterator migration, see [Migration Guide: Gorums v2 to v3](migration-v2-to-v3.md).

## API map

| v0.11 | v0.13 |
| --- | --- |
| `gorums.Configuration` | `gorums.Config` |
| `NewManager`, `NewConfiguration(mgr, source)` | `NewConfig(source, opts...)`; derive configurations with `Config` methods |
| `cfg, err := NewConfig(...)` | `cfg, closeFn, err := gorums.NewConfig(source, opts...)` |
| `cfg.Manager().Close()` or `mgr.Close()` | `closeFn()` returned by `NewConfig` |
| `WithDialOptions(...)` | `WithGRPCDialOptions(...)` |
| `WithNodeMap(...)` | `WithNodes(...)`, with values implementing `NodeAddress` |
| `cfg.Context(ctx)`, `pb.ReadQC(cfgCtx, req)` | `pb.NewStorageClient(cfg).ReadQC(ctx, req)` |
| `node.Context(ctx)`, `pb.ReadRPC(nodeCtx, req)` | `pb.NewStorageNodeClient(node).ReadRPC(ctx, req)` |
| `gorums.ServerCtx` | `gorums.ServerContext` |
| `responses.Seq()` | `responses.Results()` |
| Call options with `gorums.Interceptors(...)` | Typed interceptors on the call handle with `.Intercept(...)` |
| `pb.WriteMulticast(cfgCtx, req)` | `pb.NewStorageClient(cfg).WriteMulticast(ctx, req).Send()` |

Generated package aliases and constructors have changed too.
Use the `gorums` package to create configurations, then use the generated client for each service.
For configuration changes, `Remove`, `Difference`, `Union`, and `Extend` replace the corresponding manager-based construction patterns.
`Extend` can return an error; the other three return a new configuration.
See [Configuration Management](user-guide.md#working-with-configurations) for the current operations.

## Create and close a configuration

In v0.11, the manager owned the connections:

```go
cfg, err := gorums.NewConfig(
    gorums.WithNodeList(addrs),
    gorums.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
)
if err != nil {
    return err
}
defer cfg.Manager().Close()
```

In v0.13, keep the close function returned by `NewConfig`:

```go
cfg, closeFn, err := gorums.NewConfig(
    gorums.WithNodeList(addrs),
    gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
)
if err != nil {
    return err
}
defer closeFn()
```

The source is the first argument and dial options follow it.
The close function owns the connection pool shared by `cfg` and configurations derived from it, including nodes added with `Extend`.
Call it when all users of that pool have finished.
Generated clients do not own or close the configuration.

## Use generated service clients

Create a configuration client for quorum calls and multicasts:

```go
// v0.11
reply, err := pb.ReadQC(cfg.Context(ctx), req).Majority()

// v0.13
client := pb.NewStorageClient(cfg)
reply, err := client.ReadQC(ctx, req).Majority()
```

Create a node client for remote calls and unicasts:

```go
// v0.11
reply, err := pb.ReadRPC(node.Context(ctx), req)

// v0.13
reply, err := pb.NewStorageNodeClient(node).ReadRPC(ctx, req)
```

The constructor takes the node; each method takes the context and request.
Build a node client where it is needed, including inside a loop over nodes.
A service gets only the client types needed by its call types.
Build a new configuration client after deriving a different configuration.
The generated call functions, `ConfigContext`, `NodeContext`, and their `Context` methods are gone.
A nil node returns `gorums.ErrNoNode` on a node call, and an empty configuration returns `gorums.ErrEmptyConfig` on a quorum call or multicast.

## Dispatch and collect calls

A generated quorum call returns a typed `*gorums.Call[Req, Resp]` handle.
Creating the handle does not send the request.
A terminal operation such as `First`, `Majority`, `All`, `Threshold`, or `Correctable` dispatches the call and consumes its responses.
`Results` marks the handle for consumption; iterating its sequence dispatches the call.
Use one terminal operation per call; create a new handle for another call.
`Results()` returns a single-use sequence for custom aggregation.
`Majority` waits for a majority of successful responses and returns the first successful response, not an application-specific merge.

To dispatch now and collect later, use `Async` once, then one terminal operation on its returned responses:

```go
responses := client.ReadQC(ctx, req).Async()
// Do other work, or dispatch another call.
reply, err := responses.Majority()
```

`Async` starts no goroutine to collect replies.
Apply typed client interceptors with `.Intercept(...)` before dispatch; `gorums.CallContext[Req, Resp]` replaces `gorums.ClientCtx[Req, Resp]` in custom interceptors.
`MapRequest` and `MapResponse` are typed interceptors.
For details on aggregation, streaming, and interceptors, see [Quorum Calls](user-guide.md#quorum-calls).

One-way calls also return handles and must be dispatched:

```go
// v0.11
err := pb.WriteMulticast(cfg.Context(ctx), req)

// v0.13: send and wait for the sends
err := client.WriteMulticast(ctx, req).Send()

// Or dispatch now and wait later.
h := client.WriteMulticast(ctx, req).Async()
err = h.Wait()
```

`Send` is equivalent to `Async().Wait()`.
`Wait` reports send errors after messages reach their node streams; it does not confirm that a server handler completed.
An async one-way handle may be dropped, but its send errors are then lost.
Unicast follows the same pattern through a node client.
Do not call `Send` or `Async` twice on the same call handle.

## Server lifecycle and peer calls

A handler-only server still uses `NewServer`, service registration, `Serve`, and `Stop` or `GracefulStop`.
Change handler parameters from `gorums.ServerCtx` to `gorums.ServerContext`.
For a streaming quorum handler, change `send func(*Response) error` and the handler's `error` return to `send func(*Response)` and no return value.
Regenerate the service interface and update each implementation to match it.

For server-to-server calls, configure peers on the server:

```go
srv := gorums.NewServer(
    gorums.WithPeers(myID, gorums.WithNodeList(addrs),
        gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials()))),
)
pb.RegisterStorageServer(srv, implementation)
defer srv.Stop()
// Serve a listener with srv.Serve(listener), or use WithAddr and ListenAndServe.
```

Every server must agree on the ID for each peer, and `myID` must identify this server in the source.
`WithNodeList` assigns IDs in list order starting at 1, so use the same list order at every server.
`WithPeers` includes the local node in `srv.PeerConfig()` and in `ctx.PeerConfig()` inside handlers; local calls run in process.
The server owns these peer connections and closes them when it stops.

For a nested call, build a client from the handler's peer configuration.
Dispatch before `Release` to preserve per-sender FIFO order, and wait after it so later requests can start:

```go
func (s *storageServer) ReadQC(ctx gorums.ServerContext, req *pb.ReadRequest) (*pb.ReadResponse, error) {
    responses := pb.NewStorageClient(ctx.PeerConfig()).ReadQC(ctx, req).Async()
    ctx.Release()
    return responses.Majority()
}
```

This is a dispatch pattern; use an appropriate peer method to avoid recursively calling the same handler.
`ctx.ConnectedClients()` and `srv.ConnectedClients()` instead return the currently connected clients that can receive back-channel calls.
A client must provide a server with registered handlers through `gorums.WithBackChannel(clientServer)` when it creates its configuration.
Ordinary clients are not included.
See [Server-to-Server Communication](user-guide.md#nested-quorum-calls-with-servercontextpeerconfig) and [Back-Channel Communication](user-guide.md#reverse-direction-calls-with-servercontextconnectedclients) for setup and lifecycle details.

## Regenerate and build

Update the Gorums runtime dependency and install the matching `protoc-gen-gorums` version.
Before v0.13 is tagged, use the intended release commit for both rather than `@latest`.
Run your project's normal `protoc` or `buf` generation command for every Gorums `.proto` file.
Check in the regenerated `*_gorums.pb.go` files and the new `gorums.pb.go` generated once per Go package.
A project-specific `make genproto` is suitable only if it invokes the updated plugin for all of those files.

Generated code has compile-time version checks against the runtime's `MinVersion` and `MaxVersion`.
The generator embeds its required `GenVersion`; code below the runtime minimum or requiring a newer runtime fails to compile.
Regenerate old code with the matching plugin, or update the runtime when generated code requires a newer one.
The release process sets these version constants together; do not edit generated checks by hand.

A unicast or multicast must return `google.protobuf.Empty` in its `.proto` declaration.
The generator now rejects other response types.
The new package file reserves `Config`, `Correctable`, `ID`, and `Node`; generated service client and server names are also reserved.
Rename conflicting declarations, including hand-written Go declarations, and rename a local `.proto` named `gorums.proto` if its output would collide with the package file.
See [Function API to Service Clients](migration-v2-to-v3.md#function-api-to-service-clients) for further generator changes.

## Downstream checklist

- [ ] Update the runtime and generator together; regenerate all Gorums packages and include `gorums.pb.go`.
- [ ] Replace manager-based configuration creation, options, and configuration transformations.
- [ ] Keep and call each `NewConfig` close function after all users of its pool finish.
- [ ] Replace generated functions and configuration or node contexts with service clients.
- [ ] Dispatch each quorum or one-way handle and consume its result once; migrate interceptors and custom response iterators.
- [ ] Update server handler signatures, peer setup, back-channel clients, and shutdown ownership.
- [ ] Fix proto response types and generated-name collisions reported by the generator.
- [ ] Build and run the downstream test suite, including peer calls, one-way send errors, and shutdown.
