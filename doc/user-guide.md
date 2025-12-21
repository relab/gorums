# User Guide

You may wish to read the gRPC [Getting Started documentation](http://www.grpc.io/docs/) before continuing.
Gorums uses gRPC under the hood, and exposes some of its configuration.
Gorums also uses [Protocol Buffers](https://developers.google.com/protocol-buffers) to specify messages and RPC methods.

## Prerequisites

This guide describes how to use Gorums as a user.
The guide requires a working Go installation.
At least Go version 1.16 is required.

There are a few tools that need to be installed first:

1. Install version 3 of `protoc`, the Protocol Buffers Compiler.
   Installation of this tool is OS/distribution specific.

   On Linux/macOS/WSL with Homebrew you can use:

   ```shell
   brew install protobuf
   ```

   See the [releases](https://github.com/google/protobuf/releases) page for details and other releases.

2. Install the [Go code generator](https://github.com/protocolbuffers/protobuf-go) for `protoc`.
   It can be installed with the following command:

   ```shell
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   ```

3. Install the Gorums plugin:

   ```shell
   go install github.com/relab/gorums/cmd/protoc-gen-gorums@latest
   ```

## Creating and Compiling a Protobuf Service Description into Gorums Code

In this example we will create a simple storage service.
The storage can store a single `{value, timestamp}` tuple with methods for reading and writing state.

First, we define our storage as a gRPC service by using the protocol buffers interface definition language (IDL).
Refer to the protocol buffers [language guide](https://developers.google.com/protocol-buffers/docs/proto3) to learn more about the Protobuf IDL.

Let's create a file, `storage.proto`, in a new Go package called `gorumsexample`.
We will use `$HOME/gorumsexample` as the project root, and we will use the Go module system:

```shell
mkdir $HOME/gorumsexample
cd $HOME/gorumsexample
go mod init gorumsexample
go get github.com/relab/gorums
```

### Call Types

Gorums offers several call types including synchronous quorum calls, and one-way `unicast` and `multicast` communication.
To select a call type for a Protobuf service method, specify one of the following options (they cannot be combined):

| Call type   | Gorums option       | Description                                                       |
| ----------- | ------------------- | ----------------------------------------------------------------- |
| Ordered RPC | no option           | FIFO-ordered synchronous RPC to a single node.                    |
| Unicast     | `gorums.unicast`    | FIFO-ordered one-way asynchronous unicast.                        |
| Multicast   | `gorums.multicast`  | FIFO-ordered one-way asynchronous multicast.                      |
| Quorum Call | `gorums.quorumcall` | FIFO-ordered synchronous quorum call on a configuration of nodes. |

The generated API is similar to unary gRPC, unless the `stream` keyword is used in the proto definition.
Server streaming is only supported for quorum calls.
Client streaming is not supported.

### Configuring Call Types in Protobuf

The file `storage.proto` should have the following content, illustrating the different call types:

```proto
edition = "2024";

package gorumsexample;
option go_package = "github.com/relab/gorums/examples/gorumsexample";
option features.field_presence = IMPLICIT;

import "google/protobuf/empty.proto";
import "gorums.proto";

// Storage service defines the RPCs for a simple key-value storage system.
service Storage {
  // ReadOrdered is a FIFO-ordered RPC to a single node.
  rpc ReadOrdered(google.protobuf.Empty) returns (State) {}

  // WriteUnicast is an asynchronous unicast to a single node.
  // No reply is collected.
  rpc WriteUnicast(State) returns (google.protobuf.Empty) {
    option (gorums.unicast) = true;
  }

  // WriteMulticast is an asynchronous multicast to all nodes in a configuration.
  // No replies are collected.
  rpc WriteMulticast(State) returns (google.protobuf.Empty) {
    option (gorums.multicast) = true;
  }

  // ReadQC is a FIFO-ordered synchronous quorum call.
  // Use terminal methods (.Majority(), .First(), etc.) to retrieve results.
  // Use .AsyncMajority() for async variant, .Correctable(n) for correctable variant.
  rpc ReadQC(google.protobuf.Empty) returns (State) {
    option (gorums.quorumcall) = true;
  }

  // ReadQCStream is a FIFO-ordered synchronous quorum call with server streaming.
  // The stream keyword enables correctable calls where nodes can send multiple
  // progressive updates. Use .Correctable(n) to watch for updates as they arrive.
  rpc ReadQCStream(google.protobuf.Empty) returns (stream State) {
    option (gorums.quorumcall) = true;
  }
}

// State represents the value and its timestamp stored in a node.
message State {
  string value = 1;
  int64 timestamp = 2;
}
```

For the `unicast` and `multicast` call types, the response message type will be unused by Gorums.

### Compiling the Service Definition

Next, we compile our service definition into Go code which includes:

1. Go code to access and manage the defined Protobuf messages.
2. A Gorums client API and server interface for the storage.

We simply invoke `protoc` to compile our Protobuf definition:

```shell
protoc -I="$(go list -m -f {{.Dir}} github.com/relab/gorums):." \
  --go_out=paths=source_relative:. \
  --gorums_out=paths=source_relative:. \
  storage.proto
```

The above step should produce two files named `storage.pb.go` and `storage_gorums.pb.go` in your package directory.
The former contains the Protobuf definitions of our messages.
The latter contains the Gorums generated client and server interfaces.

### Examining the Gorums Generated Code

Let us examine the `storage_gorums.pb.go` file to see the code generated from our Protobuf definitions.
The client functions below are generated by Gorums based on the call type specified in the Protobuf definition.
The first two functions are used to send requests to a single node determined by the `NodeContext`, while the last three are used to send requests to a configuration of nodes determined by the `ConfigContext`.
The last two functions return a `*gorums.Responses[*State]` object, which is a collection of responses from the nodes in the configuration.

```go
func ReadOrdered(ctx *gorums.NodeContext, in *emptypb.Empty) (resp *State, err error)
func WriteUnicast(ctx *gorums.NodeContext, in*State, opts ...gorums.CallOption) error
func WriteMulticast(ctx *gorums.ConfigContext, in *State, opts ...gorums.CallOption) error
func ReadQC(ctx *gorums.ConfigContext, in *emptypb.Empty, opts ...gorums.CallOption) *gorums.Responses[*State]
func ReadQCStream(ctx *gorums.ConfigContext, in *emptypb.Empty, opts ...gorums.CallOption) *gorums.Responses[*State]
```

And this is our server interface:

```go
type StorageServer interface {
	ReadOrdered(ctx gorums.ServerCtx, request *emptypb.Empty) (response *State, err error)
	WriteUnicast(ctx gorums.ServerCtx, request *State)
	WriteMulticast(ctx gorums.ServerCtx, request *State)
	ReadQC(ctx gorums.ServerCtx, request *emptypb.Empty) (response *State, err error)
	ReadQCStream(ctx gorums.ServerCtx, request *emptypb.Empty, send func(response *State) error) error
}
```

**Note:**
You may decide to keep the `.proto` file and the generated `.pb.go` files in a separate directory/package and import that package (the generated Gorums API) into your application.
We skip that here for the sake of simplicity.

## Implementing the StorageServer

We now describe how to implement the `StorageServer` interface from above.

```go
type storageSrv struct {
  mut   sync.Mutex
  state *State
}

func (srv *storageSrv) ReadOrdered(_ gorums.ServerCtx, req *emptypb.Empty) (resp *State, err error) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  return srv.state, nil
}

func (srv *storageSrv) WriteUnicast(_ gorums.ServerCtx, req *State) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  if srv.state.GetTimestamp() < req.GetTimestamp() {
    srv.state = req
  }
}

func (srv *storageSrv) WriteMulticast(_ gorums.ServerCtx, req *State) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  if srv.state.GetTimestamp() < req.GetTimestamp() {
    srv.state = req
  }
}

func (srv *storageSrv) ReadQC(_ gorums.ServerCtx, req *emptypb.Empty) (resp *State, err error) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  return srv.state, nil
}

func (srv *storageSrv) ReadQCStream(_ gorums.ServerCtx, req *emptypb.Empty, send func(response *State) error) error {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  return send(srv.state)
}
```

There are some important things to note about implementing the server interfaces:

* The handlers run in the order messages are received.
* Messages from the same sender are executed in FIFO order at all servers.
  See the auxiliary documentation for more information about [message ordering](./ordering.md) in Gorums.
* Messages from different senders may be received in a different order at the different servers.
  To guarantee messages from different senders are executed in-order at the different servers, you must use a total ordering protocol.
* Errors should be returned using the [`status` package](https://pkg.go.dev/google.golang.org/grpc/status?tab=doc).
* Handlers run synchronously, and hence a long-running handler will prevent other messages from being handled.
  To help solve this problem, our `ServerCtx` objects have a `Release()` function that releases the handler's lock on the server,
  which allows the next request to be processed. After `ctx.Release()` has been called, the handler may run concurrently
  with the handlers for the next requests. The handler automatically calls `ctx.Release()` after returning.

  ```go
  func (srv *storageSrv) ReadOrdered(ctx gorums.ServerCtx, req *emptypb.Empty) (resp *State, err error) {
    // any code running before this will be executed in-order
    ctx.Release()
    // after Release() has been called, a new request handler may be started,
    // and thus it is not guaranteed that the replies will be sent back the same order.
    srv.mut.Lock()
    defer srv.mut.Unlock()
    return srv.state, nil
  }
  ```

* The context passed to the handlers is the gRPC stream context of the underlying gRPC stream.
  This context can be used to retrieve [metadata](https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md)
  and [peer](https://godoc.org/google.golang.org/grpc/peer) information from the client.

To start the server, we need to create a *listener* and a *GorumsServer*, and then register our server implementation:

```go
func ExampleStorageServer(port int) {
  lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
  if err != nil {
    log.Fatal(err)
  }
  gorumsSrv := gorums.NewServer()
  srv := storageSrv{state: &State{}}
  RegisterStorageServer(gorumsSrv, &srv)
  gorumsSrv.Serve(lis)
}
```

## Implementing the StorageClient

Next, we write client code to call RPCs on our servers.
The first thing we need to do is to create an instance of the `Manager` type.
The manager maintains a pool of connections to nodes.
Nodes are added to the connection pool via new configurations, as shown below.

The manager takes as arguments a set of optional manager options.
We can forward gRPC dial options to the manager if needed.
The manager will use these options when connecting to nodes.
Below we use only a simple insecure connection option.

```go
package gorumsexample

import (
  "log"

  "github.com/relab/gorums"
  "google.golang.org/grpc"
  "google.golang.org/grpc/credentials/insecure"
)

func ExampleStorageClient() {
  mgr := NewManager(
    gorums.WithDialOptions(
      grpc.WithTransportCredentials(insecure.NewCredentials()),
    ),
  )
```

A configuration is a set of nodes on which our RPC calls can be invoked.
Using the `WithNodeList` option, the manager assigns a unique identifier to each node.
The code below shows how to create a configuration:

```go
  // Get all all available node ids, 3 nodes
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }
  // Create a configuration including all nodes
  allNodesConfig, err := NewConfiguration(mgr, gorums.WithNodeList(addrs))
  if err != nil {
    log.Fatalln("error creating read config:", err)
  }
```

The `Manager` and `Configuration` types also have few other available methods.
Inspect the package documentation or source code for details.

We can now invoke the WriteUnicast RPC on each `node` in the configuration:

```go
  state := &State{
    Value:     proto.String("42"),
    Timestamp: proto.Int64(time.Now().Unix()),
  }

  // Invoke WriteUnicast RPC on all nodes in config
  ctx := context.Background()
  for _, node := range allNodesConfig.Nodes() {
    nodeCtx := node.Context(ctx)
    err := WriteUnicast(nodeCtx, state)
    if err != nil {
      log.Fatalln("write rpc returned error:", err)
    }
  }
```

While Gorums allows us to call RPCs on individual nodes as we did above, Gorums also provides call types *multicast* and *quorum call* that allow us to invoke an RPC on all nodes in a configuration with a single invocation, as we show in the next section.

## Quorum Calls

Instead of invoking an RPC explicitly on all nodes in a configuration, Gorums allows users to invoke a *quorum call* that sends RPCs to all nodes in parallel and collects responses.

Specifying the `quorumcall` option for RPC methods:

```protobuf
rpc ReadQC(google.protobuf.Empty) returns (State) {
  option (gorums.quorumcall) = true;
}
```

The generated code provides a function for each quorum call method:

```go
func ReadQC(ctx *gorums.ConfigContext, in *emptypb.Empty, opts ...gorums.CallOption) *gorums.Responses[*State]
```

This function returns a `*gorums.Responses[*State]` object that provides several ways to aggregate and process responses.

### Terminal Methods for Response Aggregation

The `*gorums.Responses[T]` type provides built-in *terminal methods* for common aggregation patterns:

| Method          | Description               | Returns When                         |
| --------------- | ------------------------- | ------------------------------------ |
| `.First()`      | First successful response | Any node responds successfully       |
| `.Majority()`   | Majority of responses     | ⌈(n+1)/2⌉ nodes respond successfully |
| `.All()`        | All responses             | All n nodes respond                  |
| `.Threshold(n)` | At least n responses      | n nodes respond successfully         |

Each terminal method blocks until the threshold is met or the context is canceled, then returns a single aggregated response and an error.

### Using Terminal Methods

```go
func ExampleTerminalMethods(config *Configuration) {
  ctx := context.Background()
  cfgCtx := config.Context(ctx)

  // Fast reads: return first successful response
  reply, err := ReadQC(cfgCtx, &emptypb.Empty{}).First()

  // Crash fault tolerance: require simple majority
  reply, err = ReadQC(cfgCtx, &emptypb.Empty{}).Majority()

  // Wait for all nodes (useful for debugging)
  reply, err = ReadQC(cfgCtx, &emptypb.Empty{}).All()

  // Custom threshold (e.g., f+1 for crash tolerance)
  f := 1
  reply, err = ReadQC(cfgCtx, &emptypb.Empty{}).Threshold(f + 1)
}
```

Terminal methods return `gorums.ErrIncomplete` when:

* The context is canceled before the threshold is met
* Not enough successful responses are received
* All nodes return errors

### Async and Correctable Variants

Quorum calls support asynchronous and correctable variants through additional *terminal methods*.

**Async variants** return a future (`*Async[T]`) that can be awaited later:

| Method               | Description          | Returns     |
| -------------------- | -------------------- | ----------- |
| `.AsyncFirst()`      | First response async | `*Async[T]` |
| `.AsyncMajority()`   | Majority async       | `*Async[T]` |
| `.AsyncAll()`        | All responses async  | `*Async[T]` |
| `.AsyncThreshold(n)` | Threshold async      | `*Async[T]` |

```go
future := ReadQC(cfgCtx, &emptypb.Empty{}).AsyncMajority()
// Do other work...
reply, err := future.Get()
```

**Correctable variant** provides progressive updates as more responses arrive:

| Method            | Description                | Returns           |
| ----------------- | -------------------------- | ----------------- |
| `.Correctable(n)` | Progressive updates from n | `*Correctable[T]` |

```go
corr := ReadQCStream(cfgCtx, &emptypb.Empty{}).Correctable(2)  // Initial threshold
reply, level, err := corr.Get()
<-corr.Watch(3)  // Wait for higher level
reply, level, err = corr.Get()
```

## Iterator-Based Custom Aggregation

For complex aggregation logic beyond the built-in terminal methods, use the iterator API provided by `responses.Seq()`.
The iterator yields responses progressively as they arrive, allowing custom decision-making logic.

### Basic Iterator Pattern

```go
func newestValue(responses *gorums.Responses[*State]) (*State, error) {
  var newest *State
  for resp := range responses.Seq() {
    // resp.Value contains the response message (may be nil if resp.Err is set)
    // resp.Err contains any error from that node (nil if successful)
    // resp.NodeID contains the node identifier

    if resp.Err != nil {
      continue  // Skip failed responses
    }

    if newest == nil || resp.Value.GetTimestamp() > newest.GetTimestamp() {
      newest = resp.Value
    }
  }

  if newest == nil {
    return nil, gorums.ErrIncomplete
  }
  return newest, nil
}

// Usage
cfgCtx := config.Context(ctx)
reply, err := newestValue(ReadQC(cfgCtx, &ReadRequest{}))
```

### Iterator Helper Methods

The `ResponseSeq[T]` type provides helper methods for common operations:

#### `IgnoreErrors()` - Skip Failed Responses

```go
func countSuccessful(responses *gorums.Responses[*Response]) int {
  count := 0
  for resp := range responses.Seq().IgnoreErrors() {
    count++
    // resp.Value is guaranteed to be non-nil
    // resp.Err is guaranteed to be nil
  }
  return count
}
```

#### `Filter()` - Custom Filtering

```go
func validResponses(responses *gorums.Responses[*Response]) []*Response {
  var valid []*Response

  // Only include responses that pass validation
  filtered := responses.Seq().Filter(func(nr gorums.NodeResponse[*Response]) bool {
    return nr.Err == nil && isValid(nr.Value)
  })

  for resp := range filtered {
    valid = append(valid, resp.Value)
  }
  return valid
}
```

#### `CollectN()` and `CollectAll()` - Collect Multiple Responses

```go
func majorityWrite(responses *gorums.Responses[*WriteResponse]) (*WriteResponse, error) {
  majority := (responses.Size() + 1) / 2

  // Collect first 'majority' successful responses
  replies := responses.Seq().IgnoreErrors().CollectN(majority)

  if len(replies) < majority {
    return nil, gorums.ErrIncomplete
  }

  // Process the map[uint32]*WriteResponse
  return aggregateWrites(replies), nil
}

// CollectAll waits for all responses
func allResponses(responses *gorums.Responses[*Response]) map[uint32]*Response {
  return responses.Seq().CollectAll()
}

// Combine with IgnoreErrors to collect only successful responses
func allSuccessful(responses *gorums.Responses[*Response]) map[uint32]*Response {
  return responses.Seq().IgnoreErrors().CollectAll()
}
```

### Complete Example: Storage Client with Custom Aggregation

```go
func ExampleStorageClient() {
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }

  mgr := gorums.NewManager(
    gorums.WithDialOptions(
      grpc.WithTransportCredentials(insecure.NewCredentials()),
    ),
  )

  // Create a configuration with all nodes
  cfg, err := NewConfiguration(mgr, gorums.WithNodeList(addrs))
  if err != nil {
    log.Fatalln("error creating configuration:", err)
  }

  ctx := context.Background()
  cfgCtx := config.Context(ctx)

  // Option 1: Use custom aggregation function
  reply, err := newestValue(ReadQC(cfgCtx, &ReadRequest{Key: "x"}))
  if err != nil {
    log.Fatalln("read quorum call returned error:", err)
  }
  fmt.Printf("Read value: %v\n", reply.GetValue())

  // Option 2: Use built-in terminal method
  reply, err = ReadQC(cfgCtx, &ReadRequest{Key: "x"}).Majority()
  if err != nil {
    log.Fatalln("read quorum call returned error:", err)
  }
  fmt.Printf("Read value: %v\n", reply.GetValue())
}

// newestValue returns the response with the most recent timestamp
func newestValue(responses *gorums.Responses[*State]) (*State, error) {
  var newest *State
  for resp := range responses.Seq() {
    if resp.Err != nil {
      continue
    }
    if newest == nil || resp.Value.GetTimestamp() > newest.GetTimestamp() {
      newest = resp.Value
    }
  }
  if newest == nil {
    return nil, gorums.ErrIncomplete
  }
  return newest, nil
}
```

### Early Termination with Iterators

Iterators allow early termination once a condition is met:

```go
func fastQuorum(responses *gorums.Responses[*Response]) (*Response, error) {
  const threshold = 2
  count := 0

  for resp := range responses.Seq().IgnoreErrors() {
    count++
    if count >= threshold {
      return resp.Value, nil  // Return immediately after threshold
    }
  }

  return nil, gorums.ErrIncomplete
}
```

## Custom Return Types

Gorums supports custom aggregation functions that return types different from the proto response type.
This is useful when you need to aggregate multiple responses into a summary, statistics object, or any other custom type.

### Recommended Pattern: Functions Taking `*Responses[Resp]`

The recommended approach is to define functions that accept `*Responses[Resp]` directly.
This gives you full access to all iterator methods (`IgnoreErrors()`, `Filter()`, `CollectN()`, `CollectAll()`) and the ability to return any type.

```go
// Custom aggregation function that returns a different type
// Input: *Responses[*MemoryStat], Output: *MemoryStatList
func CollectStats(resp *gorums.Responses[*MemoryStat]) (*MemoryStatList, error) {
  replies := resp.IgnoreErrors().CollectAll()
  if len(replies) == 0 {
    return nil, gorums.ErrIncomplete
  }
  return &MemoryStatList{
    MemoryStats: slices.Collect(maps.Values(replies)),
  }, nil
}

// Usage: Call the function directly, passing the Responses object
cfgCtx := config.Context(ctx)
memStats, err := CollectStats(StopBenchmark(cfgCtx, &StopRequest{}))
```

### Example: Same Type Aggregation

When the return type matches the response type, you can still use this pattern for custom quorum logic:

```go
// Custom majority quorum with validation
func ValidatedMajority(resp *gorums.Responses[*State]) (*State, error) {
  replies := resp.IgnoreErrors().CollectN(resp.Size()/2 + 1)
  if len(replies) < resp.Size()/2+1 {
    return nil, gorums.ErrIncomplete
  }
  // Return the first valid reply
  for _, r := range replies {
    if isValid(r) {
      return r, nil
    }
  }
  return nil, gorums.ErrIncomplete
}

// Usage
cfgCtx := config.Context(ctx)
state, err := ValidatedMajority(ReadQC(cfgCtx, &ReadRequest{}))
```

### Example: Custom Return Type (Slice)

```go
// Collect all string values from responses
func CollectAllValues(resp *gorums.Responses[*StringValue]) ([]string, error) {
  replies := resp.IgnoreErrors().CollectAll()
  if len(replies) == 0 {
    return nil, gorums.ErrIncomplete
  }
  result := make([]string, 0, len(replies))
  for _, v := range replies {
    result = append(result, v.GetValue())
  }
  return result, nil
}

// Usage: Returns []string instead of *StringValue
values, err := CollectAllValues(GetValues(cfgCtx, &Request{}))
```

### Example: Computing Aggregate Statistics

```go
// Aggregate results from multiple nodes into a summary
func AggregateResults(resp *gorums.Responses[*Result]) (*Result, error) {
  replies := resp.IgnoreErrors().CollectAll()
  if len(replies) == 0 {
    return nil, gorums.ErrIncomplete
  }

  summary := &Result{}
  for _, reply := range replies {
    summary.TotalOps += reply.TotalOps
    summary.TotalTime += reply.TotalTime
    summary.Throughput += reply.Throughput
  }

  // Calculate averages
  n := uint64(len(replies))
  summary.TotalOps /= n
  summary.TotalTime /= int64(n)
  summary.Throughput /= float64(n)

  return summary, nil
}
```

### Example: Returning a Primitive Type

```go
// Count responses from specific nodes
func CountFromPrimaryNodes(resp *gorums.Responses[*Response]) (int, error) {
  count := 0
  for r := range resp.IgnoreErrors().Filter(func(nr gorums.NodeResponse[*Response]) bool {
    return isPrimaryNode(nr.NodeID)
  }) {
    count++
  }
  if count == 0 {
    return 0, gorums.ErrIncomplete
  }
  return count, nil
}
```

### Example: Explicit Error Handling

When you need to handle errors from individual nodes explicitly:

```go
// Require all nodes to succeed
func RequireAllSuccess(resp *gorums.Responses[*Response]) (*Response, error) {
  var first *Response
  for r := range resp.Seq() {
    if r.Err != nil {
      return nil, r.Err  // Fail fast on any error
    }
    if first == nil {
      first = r.Value
    }
  }
  if first == nil {
    return nil, gorums.ErrIncomplete
  }
  return first, nil
}
```

## Interceptors for Request/Response Transformation

Gorums provides interceptors to transform requests and responses on a per-node basis.
Interceptors are passed as call options and can be chained together.

### MapRequest Interceptor

Transform requests before sending to each node:

```go
cfgCtx := config.Context(ctx)
resp, err := WriteQC(cfgCtx, req,
    gorums.Interceptors(
        gorums.MapRequest(func(req *WriteRequest, node *gorums.Node) *WriteRequest {
            // Customize request for each node
            return &WriteRequest{Value: fmt.Sprintf("%s-node-%d", req.Value, node.ID())}
        }),
    ),
).Majority()
```

### MapResponse Interceptor

Transform responses received from each node:

```go
resp, err := ReadQC(cfgCtx, req,
    gorums.Interceptors(
        gorums.MapResponse(func(resp *ReadResponse, node *gorums.Node) *ReadResponse {
            // Transform response, e.g., add node ID
            resp.NodeID = node.ID()
            return resp
        }),
    ),
).Majority()
```

### Multicast/Unicast with MapRequest

```go
// Send different messages to each node in a multicast
cfgCtx := config.Context(ctx)
WriteMulticast(cfgCtx, &State{},
    gorums.Interceptors(
        gorums.MapRequest(func(msg *State, node *gorums.Node) *State {
            return &State{Value: proto.String(fmt.Sprintf("node-%d", node.ID()))}
        }),
    ),
)
```

**Note:** If `MapRequest` returns `nil` for a node, the message will not be sent to that node.

### Custom Interceptors

Beyond the built-in `MapRequest` and `MapResponse` interceptors, you can create custom interceptors for logging, filtering, or other cross-cutting concerns.

A custom interceptor has the signature:

```go
func(ctx *gorums.ClientCtx[Req, Resp], next gorums.ResponseSeq[Resp]) gorums.ResponseSeq[Resp]
```

The interceptor receives:

* `ctx` - the `ClientCtx` providing access to:
  * `.Request()` - the original request
  * `.Config()` - the configuration being used
  * `.Method()` - the RPC method name
  * `.Nodes()` - all nodes in the configuration
  * `.Node(id)` - get a specific node by ID
  * `.Size()` - number of nodes in the configuration
* `next` - the response iterator from the previous interceptor (or the default iterator)

The interceptor returns a new `ResponseSeq` that wraps `next` with custom logic.

#### Chaining Interceptors

Multiple interceptors can be passed to `gorums.Interceptors()` and are executed in order:

```go
cfgCtx := config.Context(ctx)
resp, err := ReadQC(cfgCtx, req,
    gorums.Interceptors(
        loggingInterceptor,
        gorums.MapRequest(transformFunc),
        filterInterceptor,
    ),
).Majority()
```

#### Example: Logging Interceptor

Create a logging interceptor that wraps the response iterator:

```go
func LoggingInterceptor[Req, Resp proto.Message](
    ctx *gorums.ClientCtx[Req, Resp],
    next gorums.ResponseSeq[Resp],
) gorums.ResponseSeq[Resp] {
    startTime := time.Now()
    log.Printf("[%s] Starting quorum call with request: %v", ctx.Method(), ctx.Request())

    // Wrap the response iterator to log each response
    return func(yield func(gorums.NodeResponse[Resp]) bool) {
        count := 0
        for resp := range next {
            count++
            if resp.Err != nil {
                log.Printf("[%s] Node %d error: %v", ctx.Method(), resp.NodeID, resp.Err)
            } else {
                log.Printf("[%s] Node %d response received", ctx.Method(), resp.NodeID)
            }
            if !yield(resp) {
                log.Printf("[%s] Iteration stopped after %d responses", ctx.Method(), count)
                return
            }
        }
        log.Printf("[%s] Completed: %d responses in %v", ctx.Method(), count, time.Since(startTime))
    }
}

// Usage
resp, err := ReadQC(cfgCtx, req,
    gorums.Interceptors(LoggingInterceptor[*ReadRequest, *ReadResponse]),
).Majority()
```

#### Example: Response Filtering Interceptor

Filter out responses that don't meet certain criteria:

```go
func FilterInterceptor[Req, Resp proto.Message](
    shouldInclude func(Resp) bool,
) gorums.QuorumInterceptor[Req, Resp] {
    return func(ctx *gorums.ClientCtx[Req, Resp], next gorums.ResponseSeq[Resp]) gorums.ResponseSeq[Resp] {
        return func(yield func(gorums.NodeResponse[Resp]) bool) {
            for resp := range next {
                // Skip responses that don't pass the filter
                if resp.Err == nil && !shouldInclude(resp.Value) {
                    continue
                }
                if !yield(resp) {
                    return
                }
            }
        }
    }
}

// Usage: only include responses with timestamp > threshold
threshold := time.Now().Add(-1 * time.Hour)
resp, err := ReadQC(cfgCtx, req,
    gorums.Interceptors(
        FilterInterceptor[*ReadRequest, *ReadResponse](func(r *ReadResponse) bool {
            return r.GetTimestamp().AsTime().After(threshold)
        }),
    ),
).Majority()
```

#### Example: Counting Interceptor

Count responses passing through the interceptor:

```go
func CountingInterceptor[Req, Resp proto.Message](
    counter *int,
) gorums.QuorumInterceptor[Req, Resp] {
    return func(_ *gorums.ClientCtx[Req, Resp], next gorums.ResponseSeq[Resp]) gorums.ResponseSeq[Resp] {
        return func(yield func(gorums.NodeResponse[Resp]) bool) {
            for resp := range next {
                *counter++
                if !yield(resp) {
                    return
                }
            }
        }
    }
}
```

**Note:** Custom interceptors can be defined in any package. The `ClientCtx` type and `QuorumInterceptor` signature are exported from the gorums package.

## Error Handling

Gorums provides structured error types to help you understand and handle failures in quorum calls.
The primary error type is `QuorumCallError`, which contains detailed information about which nodes failed and why.

### QuorumCallError

A `QuorumCallError` is returned when a quorum call fails.
It provides the following methods:

* **`Cause() error`** - Returns the underlying cause of the failure (e.g., `ErrIncomplete`, `ErrSendFailure`)
* **`NodeErrors() int`** - Returns the number of nodes that failed
* **`Unwrap() []error`** - Supports error unwrapping for use with `errors.Is` and `errors.As`

The error implements Go's standard error unwrapping interface, allowing `errors.Is()` and `errors.As()` to check both the direct cause and any wrapped node-specific errors.

#### Common Error Causes

Gorums defines several sentinel errors that commonly appear as the cause of a `QuorumCallError`:

* **`ErrIncomplete`** - The call could not be completed due to insufficient non-error replies to form a quorum
* **`ErrSendFailure`** - Message sending failed for one or more nodes

### Error Handling Example

Here's how to properly handle errors from a quorum call:

```go
func handleQuorumCall(cfg *gorums.Configuration, req *ReadRequest) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()

  cfgCtx := config.Context(ctx)
  reply, err := ReadQC(cfgCtx, req).Majority()

  if err != nil {
    // Check if it's a QuorumCallError
    var qcErr gorums.QuorumCallError
    if errors.As(err, &qcErr) {
      log.Printf("Quorum call failed: %v", qcErr.Cause())
      log.Printf("Failed nodes: %d", qcErr.NodeErrors())

      // Handle specific cause types
      if errors.Is(err, gorums.ErrIncomplete) {
        // Not enough replies to form a quorum
        log.Println("Insufficient responses to reach quorum")
      }
    }

    // Check for context errors
    if errors.Is(err, context.DeadlineExceeded) {
      log.Println("Quorum call timed out")
    } else if errors.Is(err, context.Canceled) {
      log.Println("Quorum call was canceled")
    }
    return
  }

  // Process successful reply
  log.Printf("Read successful: %v", reply)
}
```

### Checking for Specific Error Types

Use `errors.Is()` and `errors.As()` to check for specific error types, including both the direct cause and any node-specific errors:

```go
// Check if the error is caused by insufficient responses
if errors.Is(err, gorums.ErrIncomplete) {
  // Handle incomplete quorum
}

// Check if any node returned a specific gRPC error
if errors.Is(err, status.Error(codes.Unavailable, "")) {
  // Handle unavailable nodes
}

// Extract custom error types from node failures
var customErr MyCustomError
if errors.As(err, &customErr) {
  // Handle custom error from a node
}
```

## Handling Failed Nodes in Configurations

When a quorum call fails, you can create a new configuration that excludes the failed nodes.
The `WithoutErrors` method allows you to filter nodes based on the errors they returned:

```go
import (
  "context"
  "errors"
  "io"

  "github.com/relab/gorums"
)

// Invoke a quorum call
cfgCtx := config.Context(ctx)
state, err := ReadQC(cfgCtx, &ReadRequest{}).Majority()
if err != nil {
  var qcErr gorums.QuorumCallError
  if errors.As(err, &qcErr) {
    // Option 1: Exclude all failed nodes
	  newConfig, err := NewConfiguration(mgr, config.WithoutErrors(qcErr))

    // Option 2: Exclude only nodes with specific error types
    // For example, exclude only nodes that timed out
    newConfig, err := NewConfiguration(mgr, config.WithoutErrors(qcErr, context.DeadlineExceeded))

    // Option 3: Exclude nodes with multiple specific error types
    newConfig, err := NewConfiguration(mgr, config.WithoutErrors(qcErr,
        context.DeadlineExceeded,
        context.Canceled,
        io.EOF,
      ),
    )

    // Retry the operation with the new configuration
    newCfgCtx := newConfig.Context(ctx)
    state, err = ReadQC(newCfgCtx, &ReadRequest{}).Majority()
  }
}
```

The error type matching uses `errors.Is`, which properly handles wrapped errors.
This allows you to filter nodes based on the underlying cause of their failures, enabling fine-grained control over which nodes to exclude when creating new configurations.

## Working with Configurations

Below is an example demonstrating how to work with configurations.
These configurations are viewed from the client's perspective, and to actually make quorum calls on these configurations, there must be server endpoints to connect to.
We ignore the construction of `mgr` and error handling (except for the last configuration).

In the example below, we simply use fixed quorum sizes.

```go
func ExampleConfigClient() {
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }
  // Make configuration c1 from addrs, giving |c1| = |addrs| = 3
  c1, _ := NewConfiguration(mgr,
    gorums.WithNodeList(addrs),
  )

  newAddrs := []string{
    "127.0.0.1:9080",
    "127.0.0.1:9081",
  }
  // Make configuration c2 from newAddrs, giving |c2| = |newAddrs| = 2
  c2, _ := NewConfiguration(mgr,
    gorums.WithNodeList(newAddrs),
  )

  // Make new configuration c3 from c1 and newAddrs, giving |c3| = |c1| + |newAddrs| = 3+2=5
  c3, _ := NewConfiguration(mgr,
    c1.WithNewNodes(gorums.WithNodeList(newAddrs)),
  )

  // Make new configuration c4 from c1 and c2, giving |c4| = |c1| + |c2| = 3+2=5
  c4, _ := NewConfiguration(mgr,
    c1.And(c2),
  )

  // Make new configuration c5 from c1 except the first node from c1, giving |c5| = |c1| - 1 = 3-1 = 2
  c5, _ := NewConfiguration(mgr,
    c1.WithoutNodes(c1.NodeIDs()[0]),
  )

  // Make new configuration c6 from c3 except c1, giving |c6| = |c3| - |c1| = 5-3 = 2
  c6, _ := NewConfiguration(mgr,
    c3.Except(c1),
  )

  // Example: Handling quorum call failures and creating a new configuration
  // without failed nodes
  cfgCtx := c1.Context(ctx)
  state, err := ReadQC(cfgCtx, &ReadRequest{}).Majority()
  if err != nil {
    var qcErr gorums.QuorumCallError
    if errors.As(err, &qcErr) {
      // Create a new configuration excluding all nodes that failed
      c7, _ := NewConfiguration(mgr,
        c1.WithoutErrors(qcErr),
      )

      // Or exclude only nodes with specific error types (e.g., timeout errors)
      c8, _ := NewConfiguration(mgr,
        c1.WithoutErrors(qcErr, context.DeadlineExceeded),
      )
    }
  }
}
```
