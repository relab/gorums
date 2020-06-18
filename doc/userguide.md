# User Guide

***Note:*** this guide describes how to use the new *ordered* option in Gorums.
This option will likely become the default at some point in the future.

It may be relevant to read the gRPC "Getting Started" documentation found
[here](http://www.grpc.io/docs/) before continuing.
Gorums uses gRPC under the hood, and exposes some of its configuration.
In addition, the implementation of a Gorums server is similar to implementing a gRPC server.

## Prerequisites

This guide describes how to use Gorums as a user. The guide requires a working
Go installation and that `$GOPATH/bin` is in your `$PATH`. At least Go
version 1.13 is required.

There are a few tools that need to be installed first:

First, we need version 3 of ```protoc```, the
Protocol Buffers Compiler. Installation of this tool is
OS/distribution specific. See
<https://github.com/google/protobuf/releases> and
<https://developers.google.com/protocol-buffers>.

Second, we need to install the [Go plugin](https://github.com/protocolbuffers/protobuf-go) for `protoc`.
It can be installed with the following command:

```shell
go get google.golang.org/protobuf/cmd/protoc-gen-go
```

Finally, we can install the Gorums plugin:

```shell
go get github.com/relab/gorums/cmd/protoc-gen-gorums
```

## Creating and compiling a Protobuf service description into Gorums code

We will in this example create a very simple storage service.  The storage
can store a single `{string,timestamp}` tuple and has two methods:

* Read() State
* Write(State) Response

The first thing we should do is to define our storage as a gRPC service by
using the protocol buffers interface definition language. Let's create a file,
`storage.proto`, in a new Go package called `gorumsexample`. The
package file path may for example be

```text
$GOPATH/src/github.com/yourusername/gorumsexample
```

The file ```storage.proto``` should have the following content:

```protobuf
syntax = "proto3";

package gorumsexample;

option go_package = "github.com/<your user name>/gorumsexample";

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

Every protobuf RPC method must take and return a single protobuf message. The
```Read``` method must in this example therefore take an empty "dummy"
```ReadRequest``` as input.

We should next compile our service definition into Go code which includes:

1. Go code to access and manage the defined protobuf messages.
2. A Gorums client API and server interface for the storage.

We can now invoke ```protoc``` to compile our protobuf definition:

```shell
cd GOPATH/src/github.com/yourusername/gorumsexample
protoc -I=$GOPATH/src:. \
  --go_out=paths=source_relative:. \
  --gorums_out=paths=source_relative:. \
  storage.proto
```

You should now two files named `storage.pb.go` and `storage_gorums.pb.go` in your package directory.
The former contains the Protobuf definitions of our messages.
The latter contains the generated Gorums client API and server interface.
Our two RPC methods have the following signatures:

```go
func (n *Node) Read(ctx context.Context, in *ReadRequest) (*State, error)
func (n *Node) Write(ctx context.Context, in *State) (*WriteResponse, error)
```

**Note:** You should for a real use case keep the `proto` and generated `pb.go`
files in a separate directory and import the generate Gorums API as a sub
package into to your main application. We skip this step in this example for
the sake of simplicity.

Our server side storage interface:

```go
type Storage interface {
  Read(*ReadRequest) *State
  Write(*State) *WriteResponse
}
```

## Simple Client/Server setup

We will now describe how to use the generated Gorums API.
We begin by implementing the server interface from above:

```go
type storageSrv struct {
  mut   sync.Mutex
  state *State
}

func (srv *storageSrv) Read(req *ReadRequest) *State {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  fmt.Println("Got Read()")
  return srv.state
}

func (srv *storageSrv) Write(req *State) *WriteResponse {
  srv.mut.Lock()
  defer srv.mut.Unlock()
  if srv.state.Timestamp < req.Timestamp {
    srv.state = req
    fmt.Println("Got Write(", req.Value, ")")
    return &WriteResponse{New: true}
  }
  return &WriteResponse{New: false}
}
```

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

Now we will write client code to call RPCs on our servers.
The first thing we need to do is to create an instance of the Manager type. The Manager maintains
a connection to all the provided nodes and also keep track of every
configuration of nodes. It takes as arguments a list of node addresses and a
set of optional manager options.

We can forward gRPC dial options to the Manager if needed. The Manager will use
these options when connecting to the other nodes. Three different options are
specified in the example below.

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

A configuration is a set of nodes on which our
RPC calls can be invoked. The manager assigns every node and configuration a
unique id. The code below show how to create two different configurations:

```go
// Get all all available node ids, 3 nodes
ids := mgr.NodeIDs()

// Create a configuration including all nodes
allNodesConfig, err := mgr.NewConfiguration(ids, nil)
if err != nil {
  log.Fatalln("error creating read config:", err)
}
```

The ```Manager``` and ```Configuration``` type also have other available
methods. Se godoc or source code for details.

We can now invoke the write rpc on each of the `Nodes` in the configuration:

```go
  // Test state
  state := &State{
    Value:     "42",
    Timestamp: time.Now().Unix(),
  }

  // Invoke write RPC on all nodes in config
  for _, node := range allNodesConfig.Nodes() {
    respons, err := node.StorageClient.Write(context.Background(), state)
    if err != nil {
      log.Fatalln("read rpc returned error:", err)
    } else if !respons.New {
      log.Println("state was not new.")
    }
  }
```

While Gorums allows to call RPCs on single nodes, Gorums provides Quorum Calls
to invoke a RPC on all nodes in a configuration:

## Quorum Calls

Instead of invoking a RPC explicitly on all nodes in a configuration, Gorums
allows users to invoke the RPC as Quorum Call on the configuration.
If a RPC is invoked as Quorum Call, Gorums will invoke the RPC on all nodes in
in the configuration in parallel, collect and process replies.

For the Gorums plugin to generate quorum calls we have to specify the quorumcall option
for our RPC methods in the proto file, as shown below:

```protobuf
service QCStorage {
  rpc Read(ReadRequest) returns (State) {
    option (gorums.ordered) = true;
    option (gorums.quorumcall) = true;
   }
  rpc Write(State) returns (WriteResponse) {
    option (gorums.ordered) = true;
    option (gorums.quorumcall) = true;
   }
}
```

The generated methods have the following interface

```go
func (c *Configuration) Read(ctx context.Context, args *ReadRequest) (*State, error)
func (c *Configuration) Write(ctx context.Context, args *State) (*WriteResponse, error)
```

Gorums uses a *Quorum function* to compute the reply returned by a quorum function,
from the replies of individual servers. A Gorums quorum function has two
responsibilities:

1. Report when a set of replies form a quorum.
2. Pick a single reply from a set of replies that form a quorum.

Behind the scenes, the RPCs invoked as part of a Quorum Call return multiple
replies. Only one of these replies should be returned to the
end user. However, how such a single reply should be chosen is application
specific, and not something Gorums can generically provide a policy for. It would
be natural to for example compare message content when deciding which reply to
return to the user and often several replies have to be combined into a new one.
Gorums therefore generates a `QuorumSpec` interface, that contains a quorum
function for every quorum call. The `QuorumSpec` for generated for our example
is as follows:

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

An implementation of the ```QuorumSpec``` has to be provided by when a new
configuration is created. The example below shows an implementation of
the ```QuorumSpec```.
If not sufficiently many replies have been received yet, both quorum functions
return `false`, signaling that the quorum call should wait for further replies.
Once sufficiently many replies have been received, the `ReadQF` returns the
`*State` with the highest timestamp and `true`, signaling that the quorum call
can return. The quorum call will return the `*State` chosen by the quorum function.

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

In the following we create a configuration, including an instance of the `QSpec`
defined above and invoke a quorum call.
The quorum call will return after receiving replies from 2 servers.
The remaining, outstanding RPCs are cancelled.

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
