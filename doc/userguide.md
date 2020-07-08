# User Guide

***Note:*** This guide describes how to use the new *ordered* option in Gorums.
This option will likely become the default at some point in the future.

You may wish to read the gRPC "Getting Started" documentation found [here](http://www.grpc.io/docs/) before continuing.
Gorums uses gRPC under the hood, and exposes some of its configuration.

## Prerequisites

This guide describes how to use Gorums as a user.
The guide requires a working Go installation.
At least Go version 1.13 is required.

There are a few tools that need to be installed first:

First, we need version 3 of `protoc`, the Protocol Buffers Compiler.
Installation of this tool is OS/distribution specific.
See <https://github.com/google/protobuf/releases> and <https://developers.google.com/protocol-buffers> for details.

Second, we need to install the [Go code generator](https://github.com/protocolbuffers/protobuf-go) for `protoc`.
It can be installed with the following command:

```shell
go get google.golang.org/protobuf/cmd/protoc-gen-go
```

Finally, we can install the Gorums plugin:

```shell
go get github.com/relab/gorums/cmd/protoc-gen-gorums
```

## Creating and Compiling a Protobuf Service Description into Gorums Code

We will in this example create a very simple storage service.
The storage can store a single `{string,timestamp}` tuple and has two methods:

* `Read() State`
* `Write(State) Response`

The first thing we should do is to define our storage as a gRPC service by using the protocol buffers interface definition language (IDL).
Refer to the protocol buffers [language guide](https://developers.google.com/protocol-buffers/docs/proto3) to learn more about the protobuf IDL.
Let's create a file, `storage.proto`, in a new Go package called `gorumsexample`.
We will use `$HOME/gorumsexample` as the project root, and we will use the Go module system:

```shell
mkdir $HOME/gorumsexample
cd $HOME/gorumsexample
go mod init gorumsexample
```

The file `storage.proto` should have the following content:

```protobuf
syntax = "proto3";

package gorumsexample;

option go_package = "gorumsexample";

import "github.com/relab/gorums/gorums.proto";

service Storage {
  rpc Read(ReadRequest) returns (State) {
    option (gorums.ordered) = true;
  }
  rpc Write(State) returns (WriteResponse) {
    option (gorums.ordered) = true;
  }
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

Every protobuf RPC method must take and return a single protobuf message.
The `Read` method in this example therefore take an empty `ReadRequest` as input, since no information is needed by the method.

**Note:** Having both a request and a response type is a requirement of the Protobuf IDL.
Gorums offers one-way message types through the `unicast` and `multicast` call types.
For these calltypes, the response message type will be unused by Gorums.

We should next compile our service definition into Go code which includes:

1. Go code to access and manage the defined protobuf messages.
2. A Gorums client API and server interface for the storage.

We can now invoke `protoc` to compile our protobuf definition:

```shell
cd $HOME/gorumsexample
protoc -I=. \
  --go_out=paths=source_relative:. \
  --gorums_out=paths=source_relative:. \
  storage.proto
```

You should now have two files named `storage.pb.go` and `storage_gorums.pb.go` in your package directory.
The former contains the Protobuf definitions of our messages.
The latter contains the Gorums generated client and server interfaces.
If we examine the `storage_gorums.pb.go` file, we will see the code that was generated from our protobuf definitions.
Our two RPC methods have the following signatures:

```go
func (n *Node) Read(ctx context.Context, in *ReadRequest) (*State, error)
func (n *Node) Write(ctx context.Context, in *State) (*WriteResponse, error)
```

And this is our server interface:

```go
type Storage interface {
  Read(context.Context, *ReadRequest, func(*State))
  Write(context.Context, *State, func(*WriteResponse))
}
```

**Note:** For a real use case, you may decide to keep the `.proto` file and the generated `.pb.go` files in a separate directory/package, and import that package (the generated Gorums API) into to your main application.
We skip that here for the sake of simplicity.

## Implementing the StorageServer

We will now describe how to use the generated Gorums API for the `Storage` service.
We begin by implementing the `Storage` server interface from above:

```go
type storageSrv struct {
  mut   sync.Mutex
  state *State
}

func (srv *storageSrv) Read(_ context.Context, req *ReadRequest, ret func(*State)) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  fmt.Println("Got Read()")
  ret(srv.state)
}

func (srv *storageSrv) Write(_ context.Context, req *State, ret func(*WriteResponse)) {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  if srv.state.Timestamp < req.Timestamp {
    srv.state = req
    fmt.Println("Got Write(", req.Value, ")")
    ret(&WriteResponse{New: true})
    return
  }
  ret(&WriteResponse{New: false})
}
```

There are some important things to note about implementing the server interfaces:

* Reply messages should be returned using the `ret` function.
* The handlers are run synchronously, in the order that messages are received.
  This means that a long-running handler will prevent other messages from being handled.
  However, you can start additional goroutines within each handler, provided that they return a result using the `ret` function.
  For example, the `Read` handler could be made asynchronous like this:
* The context that is passed to the handlers is the gRPC stream context of the underlying gRPC stream.
  You can use this context to retrieve [metadata](https://github.com/grpc/grpc-go/blob/master/Documentation/grpc-metadata.md)
  and [peer](https://godoc.org/google.golang.org/grpc/peer) information from the client.

  ```go
  func (srv *storageSrv) Read(_ context.Context, req *ReadRequest, ret func(*State)) {
    go func() {
      srv.mut.Lock()
      defer srv.mut.Unlock()
      fmt.Println("Got Read()")
      ret(srv.state)
    }()
  }
  ```

* It is currently not possible to send more than one reply message per request.
  This may change with other call types in the future.

For more information about message ordering and why we use channels instead of `return` in our handlers, read [ordering.md](./ordering.md)

To start the server, we need to create a *listener* and a *GorumsServer*, and then register our server implementation with the GorumsServer:

```go
func ExampleStorageServer(port int) {
  lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
  if err != nil {
    log.Fatal(err)
  }
  gorumsSrv := NewGorumsServer()
  srv := storageSrv{state: &State{}}
  gorumsSrv.RegisterStorageServer(&srv)
  gorumsSrv.Serve(lis)
}
```

## Implementing the StorageClient

Next we will write client code to call RPCs on our servers.
The first thing we need to do is to create an instance of the Manager type.
The Manager maintains a connection to all the provided nodes and also keeps track of every configuration of nodes.
It takes as arguments a list of node addresses and a set of optional manager options.

We can forward gRPC dial options to the Manager if needed.
The Manager will use these options when connecting to the other nodes.
Three different options are specified in the example below.

```go
package gorumsexample

import (
  "log"
  "time"

  "google.golang.org/grpc"
)

func ExampleStorageClient() {
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }

  mgr, err := NewManager(addrs, WithGrpcDialOptions(
    grpc.WithBlock(),
    grpc.WithInsecure(),
  ),
    WithDialTimeout(500*time.Millisecond),
  )
  if err != nil {
    log.Fatal(err)
  }
```

A configuration is a set of nodes on which our RPC calls can be invoked.
The manager assigns every node and configuration a unique identifier.
The code below show how to create two different configurations:

```go
// Get all all available node ids, 3 nodes
ids := mgr.NodeIDs()

// Create a configuration including all nodes
allNodesConfig, err := mgr.NewConfiguration(ids, nil)
if err != nil {
  log.Fatalln("error creating read config:", err)
}
```

The `Manager` and `Configuration` type also have other available methods.
See godoc or source code for details.

We can now invoke the Write RPC on each `node` in the configuration:

```go
  // Test state
  state := &State{
    Value:     "42",
    Timestamp: time.Now().Unix(),
  }

  // Invoke Write RPC on all nodes in config
  for _, node := range allNodesConfig.Nodes() {
    respons, err := node.Write(context.Background(), state)
    if err != nil {
      log.Fatalln("read rpc returned error:", err)
    } else if !respons.New {
      log.Println("state was not new.")
    }
  }
```

While Gorums allows to call RPCs on single nodes as we did above, Gorums also provides a call type _quorum call_ that allows us to invoke a RPC on all nodes in a configuration with a single invocation, as we show in the next section.

## Quorum Calls

Instead of invoking an RPC explicitly on all nodes in a configuration, Gorums allows users to invoke the RPC as a quorum call on the configuration.
If an RPC is invoked as a quorum call, Gorums will invoke the RPCs on all nodes in the configuration in parallel, and collect and process the replies.

For the Gorums plugin to generate quorum calls we have to specify the `quorumcall` option for our RPC methods in the proto file, as shown below:

```protobuf
service QCStorage {
  rpc Read(ReadRequest) returns (State) {
    option (gorums.quorumcall) = true;
    option (gorums.ordered) = true;
   }
  rpc Write(State) returns (WriteResponse) {
    option (gorums.quorumcall) = true;
    option (gorums.ordered) = true;
   }
}
```

The generated methods have the following client-side interface:

```go
func (c *Configuration) Read(ctx context.Context, args *ReadRequest) (*State, error)
func (c *Configuration) Write(ctx context.Context, args *State) (*WriteResponse, error)
```

## The QuorumSpec Interface with Quorum Functions

Gorums uses a *quorum function*, that takes as input a set of replies from individual servers, and computes the reply to be returned from the quorum call.
Such a quorum function has two responsibilities:

1. Report when a set of replies form a quorum.
2. Compute a single reply from a set of replies that form a quorum.

Behind the scenes, the RPCs invoked as part of a quorum call return multiple replies.
Typically, only one of these replies should be returned to the end user.
However, how such a single reply should be chosen is application specific, and not something Gorums can generically provide a policy for.
For example, it may be necessary to compare the content of several reply messages, when deciding which reply to return to the client's quorum call, and sometimes several replies have to be combined into a new one.
Gorums therefore generates a `QuorumSpec` interface, that contains a quorum function for every quorum call.
The `QuorumSpec` generated for our example is as follows:

```go
type QuorumSpec interface {
  // ReadQF is the quorum function for the Read
  // quorum call method.
  ReadQF(replies []*State) (*State, bool)

  // WriteQF is the quorum function for the Write
  // quorum call method.
  WriteQF(replies []*WriteResponse) (*WriteResponse, bool)
}
```

An implementation of the `QuorumSpec` has to be provided when creating a new configuration.
Each quorum function implemented as part of the `QuorumSpec` must adhere to the following rules:

* If too few replies have been received, a quorum function must return `false`, signaling to Gorums that the quorum call should wait for additional replies.
* Once sufficiently many replies have been received, the quorum function must return `true` along with an appropriate return value, signaling that the quorum call can return the value to the client.

The example below shows an implementation of the `QuorumSpec` interface.
In this example, `ReadQF` returns the `*State` with the highest timestamp and `true`, signaling that the quorum call can return.
The quorum call will return the `*State` chosen by the quorum function.

```go
package gorumsexample

import "sort"

type QSpec struct {
  quorumSize int
}

// Define a quorum function for the Read RPC method.
func (qs *QSpec) ReadQF(replies []*State) (*State, bool) {
  if len(replies) < qs.quorumSize {
    return nil, false
  }
  sort.Sort(ByTimestamp(replies))
  return replies[len(replies)-1], true
}

func (qs *QSpec) WriteQF(replies []*WriteResponse) (*WriteResponse, bool) {
  if len(replies) < qs.quorumSize {
    return nil, false
  }
  return replies[0], true
}

type ByTimestamp []*State

func (a ByTimestamp) Len() int           { return len(a) }
func (a ByTimestamp) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTimestamp) Less(i, j int) bool { return a[i].Timestamp < a[j].Timestamp }
```

## Invoking Quorum Calls on the Configuration in the StorageClient

In the following we create a configuration, including an instance of the `QSpec` defined above and invoke a quorum call.
The quorum call will return after receiving replies from two servers.
Any remaining, outstanding replies are ignored by Gorums.
Note that Gorums does not cancel any outstanding RPCs, leaving it to the client to manage any cancellations through the `Context` argument to the quorum call.
However, cancelling RPCs to a replicated server may not be what you want, since then one or more servers may not have seen previous messages.

```go
func ExampleStorageClient() {
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }

  mgr, err := NewManager(addrs, WithGrpcDialOptions(
    grpc.WithBlock(),
    grpc.WithTimeout(50*time.Millisecond),
    grpc.WithInsecure(),
  ),
  )
  if err != nil {
    log.Fatal(err)
  }

  // Get all all available node ids, 3 nodes
  ids := mgr.NodeIDs()

  // Create a configuration including all nodes
  allNodesConfig, err := mgr.NewConfiguration(ids, &QSpec{2})
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
