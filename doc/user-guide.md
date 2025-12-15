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
The storage can store a single `{string,timestamp}` tuple and has two methods:

* `Read() State`
* `Write(State) Response`

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

The file `storage.proto` should have the following content:

```proto
syntax = "proto3";
package gorumsexample;
option go_package = ".;gorumsexample";

service Storage {
  rpc Read(ReadRequest) returns (State) { }
  rpc Write(State) returns (WriteResponse) { }
}

message State {
  string Value = 1;
  int64 Timestamp = 2;
}

message WriteResponse {
  bool New = 1;
}

message ReadRequest {}
```

Every RPC method must take and return a single Protobuf message.
This is a requirement of the Protobuf IDL.
The `Read` method in this example, therefore, takes an empty `ReadRequest` as input since no information is needed by the method.

**Note:** Gorums offers one-way communication through the `unicast` and `multicast` call types.
For these call types, the response message type will be unused by Gorums.
For a detailed overview of the available method options to control the call types, see the [method options](method-options.md) document.

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
Let us examine the `storage_gorums.pb.go` file to see the code generated from our Protobuf definitions.
Our two RPC methods have the following client-side function signatures:

```go
func Read(ctx *gorums.NodeContext, in *ReadRequest) (*State, error)
func Write(ctx *gorums.NodeContext, in *State) (*WriteResponse, error)
```

And this is our server interface:

```go
type StorageServer interface {
  Read(gorums.ServerCtx, *ReadRequest) (*State, error)
  Write(gorums.ServerCtx, *State) (*WriteResponse, error)
}
```

**Note:**
For a real use case, you may decide to keep the `.proto` file and the generated `.pb.go` files in a separate directory/package and import that package (the generated Gorums API) into your application.
We skip that here for the sake of simplicity.

## Implementing the StorageServer

We now describe how to use the generated Gorums API for the `Storage` service.
We begin by implementing the `StorageServer` interface from above:

```go
type storageSrv struct {
  mut   sync.Mutex
  state *State
}

func (srv *storageSrv) Read(_ gorums.ServerCtx, req *ReadRequest) (resp *State, err error) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  fmt.Println("Got Read()")
  return srv.state, nil
}

func (srv *storageSrv) Write(_ gorums.ServerCtx, req *State) (resp *WriteResponse, err error) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  if srv.state.Timestamp < req.Timestamp {
    srv.state = req
    fmt.Println("Got Write(", req.Value, ")")
    return &WriteResponse{New: true}, nil
  }
  return &WriteResponse{New: false}, nil
}
```

There are some important things to note about implementing the server interfaces:

* The handlers run in the order messages are received.
* Messages from the same sender are executed in FIFO order at all servers.
* Messages from different senders may be received in a different order at the different servers.
  To guarantee messages from different senders are executed in-order at the different servers, you must use a total ordering protocol.
* Handlers run synchronously, and hence a long-running handler will prevent other messages from being handled.
  To help solve this problem, our `ServerCtx` objects have a `Release()` function that releases the handler's lock on the server,
  which allows the next request to be processed. After `ctx.Release()` has been called, the handler may run concurrently
  with the handlers for the next requests. The handler automatically calls `ctx.Release()` after returning.

  ```go
  func (srv *storageSrv) Read(ctx gorums.ServerCtx, req *ReadRequest) (resp *State, err error) {
    // any code running before this will be executed in-order
    ctx.Release()
    // after Release() has been called, a new request handler may be started,
    // and thus it is not guaranteed that the replies will be sent back the same order.
    srv.mut.Lock()
    defer srv.mut.Unlock()
    fmt.Println("Got Read()")
    return srv.state, nil
  }
  ```

* The context passed to the handlers is the gRPC stream context of the underlying gRPC stream.
  This context can be used to retrieve [metadata](https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md)
  and [peer](https://godoc.org/google.golang.org/grpc/peer) information from the client.
* Errors should be returned using the [`status` package](https://pkg.go.dev/google.golang.org/grpc/status?tab=doc).

For more information about message ordering and why we use channels instead of `return` in our handlers, read [ordering.md](./ordering.md).

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

We can now invoke the Write RPC on each `node` in the configuration:

```go
  // Test state
  state := &State{
    Value:     "42",
    Timestamp: time.Now().Unix(),
  }

  // Invoke Write RPC on all nodes in config
  ctx := context.Background()
  for _, node := range allNodesConfig.Nodes() {
    nodeCtx := gorums.WithNodeContext(ctx, node)
    reply, err := Write(nodeCtx, state)
    if err != nil {
      log.Fatalln("write rpc returned error:", err)
    } else if !reply.New {
      log.Println("state was not new.")
    }
  }
```

While Gorums allows us to call RPCs on individual nodes as we did above, Gorums also provides a call type *quorum call* that allows us to invoke an RPC on all nodes in a configuration with a single invocation, as we show in the next section.

## Quorum Calls

Instead of invoking an RPC explicitly on all nodes in a configuration, Gorums allows users to invoke a *quorum call* that sends RPCs to all nodes in parallel and collects responses.

For the Gorums plugin to generate quorum calls, specify the `quorumcall` option for RPC methods in the proto file:

```protobuf
import "gorums.proto";

service Storage {
  rpc ReadQC(ReadRequest) returns (State) {
    option (gorums.quorumcall) = true;
  }
  rpc WriteQC(State) returns (WriteResponse) {
    option (gorums.quorumcall) = true;
  }
}
```

The generated code provides a generic function for each quorum call method:

```go
func ReadQC(ctx *gorums.ConfigContext, in *ReadRequest, opts ...gorums.CallOption) *gorums.Responses[*State]
func WriteQC(ctx *gorums.ConfigContext, in *State, opts ...gorums.CallOption) *gorums.Responses[*WriteResponse]
```

These functions return a `*gorums.Responses[T]` object that provides several ways to aggregate and process responses.

## Terminal Methods for Response Aggregation

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
func ExampleTerminalMethods(cfg *gorums.Configuration) {
  ctx := context.Background()
  cfgCtx := gorums.WithConfigContext(ctx, cfg)

  // Fast reads: return first successful response
  reply, err := ReadQC(cfgCtx, &ReadRequest{}).First()

  // Byzantine fault tolerance: require majority
  reply, err = WriteQC(cfgCtx, &State{}).Majority()

  // Wait for all nodes (useful for debugging)
  reply, err = ReadQC(cfgCtx, &ReadRequest{}).All()

  // Custom threshold (e.g., f+1 for crash tolerance)
  f := 1
  reply, err = ReadQC(cfgCtx, &ReadRequest{}).Threshold(f + 1)
}
```

Terminal methods return `gorums.ErrIncomplete` when:

* The context is canceled before the threshold is met
* Not enough successful responses are received
* All nodes return errors

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
cfgCtx := gorums.WithConfigContext(ctx, cfg)
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
  cfgCtx := gorums.WithConfigContext(ctx, cfg)

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
This is useful when you need to aggregate multiple responses into a summary or statistics object.

### Example: Collecting Multiple Responses

Consider a `StopBenchmark` RPC that returns `*MemoryStat` from each node, but you want to return `*MemoryStatList` containing all stats:

```go
// Proto definitions:
// rpc StopBenchmark(StopRequest) returns (MemoryStat) { option (gorums.quorumcall) = true; }
// message MemoryStat { uint64 allocs = 1; uint64 memory = 2; }
// message MemoryStatList { repeated MemoryStat memory_stats = 1; }

// Custom aggregation function with different return type
func StopBenchmarkQF(replies map[uint32]*MemoryStat) (*MemoryStatList, error) {
  if len(replies) == 0 {
    return nil, gorums.ErrIncomplete
  }
  return &MemoryStatList{
    MemoryStats: slices.Collect(maps.Values(replies)),
  }, nil
}

// Usage: Two-step pattern
cfgCtx := gorums.WithConfigContext(ctx, cfg)
responses := StopBenchmark(cfgCtx, &StopRequest{})
replies := responses.Seq().IgnoreErrors().CollectAll()  // map[uint32]*MemoryStat
memStats, err := StopBenchmarkQF(replies)  // Returns *MemoryStatList
```

### Example: Computing Aggregate Statistics

```go
// Aggregate results from multiple nodes
func AggregateResults(replies map[uint32]*Result) (*Result, error) {
  if len(replies) == 0 {
    return nil, gorums.ErrIncomplete
  }

  resp := &Result{}
  for _, reply := range replies {
    resp.TotalOps += reply.TotalOps
    resp.TotalTime += reply.TotalTime
    resp.Throughput += reply.Throughput
  }

  // Calculate averages
  numNodes := len(replies)
  resp.TotalOps /= uint64(numNodes)
  resp.TotalTime /= int64(numNodes)
  resp.Throughput /= float64(numNodes)

  return resp, nil
}
```

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

  cfgCtx := gorums.WithConfigContext(ctx, cfg)
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
cfgCtx := gorums.WithConfigContext(ctx, config)
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
    newCfgCtx := gorums.WithConfigContext(ctx, newConfig)
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
  cfgCtx := gorums.WithConfigContext(ctx, c1)
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
