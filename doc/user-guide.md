# User Guide

You may wish to read the gRPC "Getting Started" documentation found [here](http://www.grpc.io/docs/) before continuing.
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

```protobuf
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

Next, we compile our service definition into Go code which includes:

1. Go code to access and manage the defined Protobuf messages.
2. A Gorums client API and server interface for the storage.

We simply invoke `protoc` to compile our Protobuf definition:

```shell
protoc -I=$(go list -m -f {{.Dir}} github.com/relab/gorums):. \
  --go_out=paths=source_relative:. \
  --gorums_out=paths=source_relative:. \
  storage.proto
```

The above step should produce two files named `storage.pb.go` and `storage_gorums.pb.go` in your package directory.
The former contains the Protobuf definitions of our messages.
The latter contains the Gorums generated client and server interfaces.
Let us examine the `storage_gorums.pb.go` file to see the code generated from our Protobuf definitions.
Our two RPC methods have the following signatures:

```go
func (n *Node) Read(ctx context.Context, in *ReadRequest) (*State, error)
func (n *Node) Write(ctx context.Context, in *State) (*WriteResponse, error)
```

And this is our server interface:

```go
type Storage interface {
  Read(context.Context, *ReadRequest, func(*State, error))
  Write(context.Context, *State, func(*WriteResponse, error))
}
```

**Note:** For a real use case, you may decide to keep the `.proto` file and the generated `.pb.go` files in a separate directory/package and import that package (the generated Gorums API) into your application.
We skip that here for the sake of simplicity.

## Implementing the StorageServer

We now describe how to use the generated Gorums API for the `Storage` service.
We begin by implementing the `Storage` server interface from above:

```go
type storageSrv struct {
  mut   sync.Mutex
  state *State
}

func (srv *storageSrv) Read(_ context.Context, req *ReadRequest, ret func(*State, error)) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  fmt.Println("Got Read()")
  ret(srv.state, nil)
}

func (srv *storageSrv) Write(_ context.Context, req *State, ret func(*WriteResponse, error)) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  if srv.state.Timestamp < req.Timestamp {
    srv.state = req
    fmt.Println("Got Write(", req.Value, ")")
    ret(&WriteResponse{New: true}, nil)
    return
  }
  ret(&WriteResponse{New: false}, nil)
}
```

There are some important things to note about implementing the server interfaces:

* Reply messages must be returned using the `ret` function.
* The handlers run in the order messages are received.
* Messages from the same sender are executed in FIFO order at all servers.
* Messages from different senders may be received in a different order at the different servers.
  To guarantee messages from different senders are executed in-order at the different servers, you must use a total ordering protocol.
* Handlers run synchronously, and hence a long-running handler will prevent other messages from being handled.
  However, you can start additional goroutines within each handler, provided that they return a result using the `ret` function.
  For example, the `Read` handler could be made asynchronous as shown below.

  ```go
  func (srv *storageSrv) Read(_ context.Context, req *ReadRequest, ret func(*State), error) {
    go func() {
      srv.mut.Lock()
      defer srv.mut.Unlock()
      fmt.Println("Got Read()")
      ret(srv.state, nil)
    }()
  }
  ```

* The context passed to the handlers is the gRPC stream context of the underlying gRPC stream.
  This context can be used to retrieve [metadata](https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md)
  and [peer](https://godoc.org/google.golang.org/grpc/peer) information from the client.
* Errors should be returned using the [`status` package](https://pkg.go.dev/google.golang.org/grpc/status?tab=doc).
* It is currently not possible to send more than one reply message per request.
  This may change with other call types in the future.

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
Three different options are specified in the example below.

```go
package gorumsexample

import (
  "log"
  "time"

  "google.golang.org/grpc"
)

func ExampleStorageClient() {
  mgr := NewManager(
    gorums.WithDialTimeout(500*time.Millisecond),
    gorums.WithGrpcDialOptions(
      grpc.WithBlock(),
      grpc.WithInsecure(),
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
  allNodesConfig, err := mgr.NewConfiguration(
    nil,
    gorums.WithNodeList(addrs),
  )
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
  for _, node := range allNodesConfig.Nodes() {
    reply, err := node.Write(context.Background(), state)
    if err != nil {
      log.Fatalln("read rpc returned error:", err)
    } else if !reply.New {
      log.Println("state was not new.")
    }
  }
```

While Gorums allows us to call RPCs on individual nodes as we did above, Gorums also provides a call type _quorum call_ that allows us to invoke an RPC on all nodes in a configuration with a single invocation, as we show in the next section.

## Quorum Calls

Instead of invoking an RPC explicitly on all nodes in a configuration, Gorums allows users to invoke a _quorum call_ via a method on the `Configuration` type.
If an RPC is invoked as a quorum call, Gorums will invoke the RPCs on all nodes in parallel and collect and process the replies.

For the Gorums plugin to generate quorum calls we need to specify the `quorumcall` option for our RPC methods in the proto file, as shown below:

```protobuf
import "gorums.proto";

service QCStorage {
  rpc Read(ReadRequest) returns (State) {
    option (gorums.quorumcall) = true;
   }
  rpc Write(State) returns (WriteResponse) {
    option (gorums.quorumcall) = true;
   }
}
```

The generated methods have the following client-side interface:

```go
func (c *Configuration) Read(ctx context.Context, in *ReadRequest) (*State, error)
func (c *Configuration) Write(ctx context.Context, in *State) (*WriteResponse, error)
```

## The QuorumSpec Interface with Quorum Functions

Gorums uses a *quorum function*, that takes as input a set of replies from individual servers, and computes the reply to be returned from the quorum call.
Such a quorum function has two responsibilities:

1. Report when a set of replies form a quorum.
2. Compute a single reply from a set of replies that form a quorum.

Behind the scenes, the RPCs invoked as part of a quorum call return multiple replies.
Typically, only one of these replies should be returned to the end-user.
However, Gorums cannot provide a generic policy for selecting a single reply from many replies.
Instead, Gorums makes this an application-specific choice.
For example, it may be necessary to compare the content of several reply messages when deciding which reply to return to the client's quorum call, and sometimes several replies must be combined into a new one.
Gorums, therefore, generates a `QuorumSpec` interface that contains a quorum function for every quorum call.
The `QuorumSpec` generated for our example is as follows:

```go
type QuorumSpec interface {
  // ReadQF is the quorum function for the Read
  // quorum call method.
  ReadQF(req *ReadRequest, replies map[uint32]*State) (*State, bool)

  // WriteQF is the quorum function for the Write
  // quorum call method.
  WriteQF(req *WriteRequest, replies map[uint32]*WriteResponse) (*WriteResponse, bool)
}
```

A `QuorumSpec` implementation must be provided when creating a new configuration.
Each quorum function of the `QuorumSpec` must adhere to the following rules:

* If too few replies have been received, a quorum function must return `false`, signaling to Gorums that the quorum call should wait for more replies.
* Once sufficiently many replies have been received, the quorum function must return `true` along with an appropriate return value, signaling that the quorum call can return the value to the client.

The example below shows an implementation of the `QuorumSpec` interface.
Here, `ReadQF` returns the `*State` with the highest timestamp and `true`, signaling that the quorum call can return.
The quorum call will return the `*State` chosen by the quorum function.

```go
package gorumsexample

import "sort"

type QSpec struct {
  quorumSize int
}

// ReadQF is the quorum function for the Read RPC method.
func (qs *QSpec) ReadQF(_ *ReadResponse, replies map[uint32]*State) (*State, bool) {
  if len(replies) < qs.quorumSize {
    return nil, false
  }
  return newestState(replies), true
}

// WriteQF is the quorum function for the Write RPC method.
func (qs *QSpec) WriteQF(_ *WriteRequest, replies map[uint32]*WriteResponse) (*WriteResponse, bool) {
  if len(replies) < qs.quorumSize {
    return nil, false
  }
  // return the first response we find
  var reply *WriteResponse
  for _, r := range replies {
    reply = r
    break
  }
  return reply, true
}

func newestState(replies map[uint32]*State) *State {
  var newest *State
  for _, s := range replies {
    if s.GetTimestamp() >= newest.GetTimestamp() {
      newest = s
    }
  }
  return newest
}
```

## Invoking Quorum Calls on the Configuration in the StorageClient

In the following code snippet, we create a configuration, including an instance of the `QSpec` defined above, and invoke a quorum call.
The quorum call will return after receiving replies from two servers.
Gorums ignores any outstanding replies.
Note that Gorums does not cancel any outstanding RPCs, leaving it to the client to manage any cancellations through the `Context` argument to the quorum call.
However, canceling RPCs to a replicated server may not be the desired behavior, since then one or more servers may not have seen previous messages.

```go
func ExampleStorageClient() {
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }

  mgr := NewManager(
    gorums.WithDialTimeout(50*time.Millisecond),
    gorums.WithGrpcDialOptions(
      grpc.WithBlock(),
      grpc.WithInsecure(),
  ))
  // Create a configuration including all nodes
  allNodesConfig, err := mgr.NewConfiguration(
    &QSpec{2},
    gorums.WithNodeList(addrs),
  )
  if err != nil {
    log.Fatalln("error creating read config:", err)
  }

  // Invoke read quorum call:
  ctx, cancel := context.WithCancel(context.Background())
  reply, err := allNodesConfig.Read(ctx, &ReadRequest{})
  if err != nil {
    log.Fatalln("read rpc returned error:", err)
  }
  cancel()
}
```
