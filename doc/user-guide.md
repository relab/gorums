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
For a detailed overview of the available method options to control the call types, see the [method options](method-options.md) document.

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
type StorageServer interface {
  Read(gorums.ServerCtx, *ReadRequest) (*State, error)
  Write(gorums.ServerCtx, *State) (*WriteResponse, error)
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
  func (srv *storageSrv) Read(ctx gorums.ServerCtx, req *ReadRequest) (resp *State), err error {
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

  "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func ExampleStorageClient() {
  mgr := NewManager(
    gorums.WithGrpcDialOptions(
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
  allNodesConfig, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
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

While Gorums allows us to call RPCs on individual nodes as we did above, Gorums also provides a call type *quorum call* that allows us to invoke an RPC on all nodes in a configuration with a single invocation, as we show in the next section.

## Quorum Calls

Instead of invoking an RPC explicitly on all nodes in a configuration, Gorums allows users to invoke a *quorum call* via a method on the `Configuration` type.
If an RPC is invoked as a quorum call, Gorums will invoke the RPCs on all nodes in parallel and return the responses.

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
func (c *QCStorageConfiguration) Read(ctx context.Context, in *ReadRequest) gorums.Responses[*State]
func (c *QCStorageConfiguration) Write(ctx context.Context, in *State) gorums.Responses[*WriteResponse]
```

## The quorum call responses

The quorum calls return a gorums.Responses object which is an iterator used to process the responses from the quorum call.
Getting the responses from the iterator can be done with a for loop just like with a slice.
Each of these responses is a struct containing the node id of the server, and contains either a proto message and an error (or both).
If you only care about the successfully received messages you can use the `IgnoreErrors` method.
We can create "quorum functions" which takes the quorum call responses and returns a single result or an error.

The example below shows an implementation of a "quorum function" for the `Read` and `Write` quorum calls.
Here, `readQF` returns the `*State` with the highest timestamp for a majority of responses.
`writeQF` returns the `*WriteResponse` from one of the responses when it received a majority of responses.
We decide to ignore all errors, meaning errors do not count towards the quorum.

```go
package gorumsexample

func readQF(responses gorums.Responses[*State], quorum int) (*State, error) {
	var newest *pb.ReadResponse
	replyCount := int(0)
	for response := range responses.IgnoreErrors() {
    if newest != nil {
		  newest = newestStateOfTwo(newest, response.Msg)
    } else {
      newest = response.Msg
    }
		replyCount++
		if replyCount >= quorum {
			return newest, nil
		}
	}
	return nil, errors.New("QCStorage.readqf: quorum not found")
}

func writeQF(responses gorums.Responses[*WriteResponse], quorum int) (*WriteResponse, error) {
	replyCount := int(0)
	for response := range responses.IgnoreErrors() {
		replyCount++
		if replyCount >= quorum {
			return response.Msg
		}
	}
	return nil, errors.New("QCStorage.writeqf: quorum not found")
}

func newestValueOfTwo(s1 *State, s2 *State) *State {
  if s1.GetTimestamp() >= s2.GetTimestamp() {
    return s1
  }
  return s2
}
```

## Invoking Quorum Calls on the Configuration in the StorageClient

In the following code snippet, we create a configuration, and invoke a quorum call.
We call a quorum function using the iterator from the quorum call.
The quorum function will return after receiving replies from two servers.
Note that Gorums does not cancel any outstanding RPCs, leaving it to the client to manage any cancellations through the `Context` argument to the quorum call.
However, canceling RPCs to a replicated server may not be the desired behavior, since then one or more servers may not have seen previous messages.

```go
func ExampleStorageClient() {
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }

  // Create a configuration including all nodes
  allNodesConfig, err := examplepb.NewQCStorageConfiguration(
    gorums.WithNodeList(addrs),
    gorums.WithGrpcDialOptions(
      grpc.WithTransportCredentials(insecure.NewCredentials()),
    ),
  )
  if err != nil {
    log.Fatalln("error creating read config:", err)
  }

  // Invoke read quorum call:
  ctx, cancel := context.WithCancel(context.Background())
  responses := allNodesConfig.Read(ctx, &ReadRequest{})
  reply, err := readQF(responses, 2)
  if err != nil {
    log.Fatalln("read rpc returned error:", err)
  }
  cancel()
}
```

## Working with Configurations

Below is an example demonstrating how to work with configurations.
These configurations are viewed from the client's perspective, and to actually make quorum calls on these configurations, there must be server endpoints to connect to.
We ignore the construction of `mgr` and error handling.

```go
func ExampleConfigClient() {
  addrs := []string{
    "127.0.0.1:8080",
    "127.0.0.1:8081",
    "127.0.0.1:8082",
  }
  // Make configuration c1 from addrs, giving |c1| = |addrs| = 3
  c1, _ := examplepb.NewExampleConfiguration(
    gorums.WithNodeList(addrs),
  )

  newAddrs := []string{
    "127.0.0.1:9080",
    "127.0.0.1:9081",
  }
  // Make subconfiguration c2 from newAddrs, giving |c2| = |newAddrs| = 2
  c2, _ := c1.SubExampleConfiguration(
    gorums.WithNodeList(newAddrs),
  )

  // Make new subconfiguration c3 from c1 and newAddrs, giving |c3| = |c1| + |newAddrs| = 3+2=5
  c3, _ := c1.SubExampleConfiguration(
    c1.WithNewNodes(gorums.WithNodeList(newAddrs)),
  )

  // Make new subconfiguration c4 from c1 and c2, giving |c4| = |c1| + |c2| = 3+2=5
  c4, _ := c1.SubExampleConfiguration(
    c1.And(c2),
  )

  // Make new subconfiguration c5 from c1 except the first node from c1, giving |c5| = |c1| - 1 = 3-1 = 2
  c5, _ := c1.SubExampleConfiguration(
    c1.WithoutNodes(c1.NodeIDs()[0]),
  )

  // Make new subconfiguration c6 from c3 except c1, giving |c6| = |c3| - |c1| = 5-3 = 2
  c6, _ := c1.SubExampleConfiguration(
    c3.Except(c1),
  )
}
```
