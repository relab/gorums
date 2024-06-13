# Broadcast

### Preliminary

The main contribution to Gorums in this Master's thesis are the following files and folders. The root directory is gorums (github.com/relab/gorums) and is not specified in the list for brevity:

- All files in this folder. (github.com/relab/gorums/broadcast)
- broadcast.go
- handler.go
- clientserver.go
- tests/broadcast
- authentication
- logging

Additionally, we have made contributions to most of the other files. These changes are presented in this draft pull request:

    https://github.com/relab/gorums/pull/176

## Documentation

We will use an example when presenting the broadcast framework. This example is Eager Reliable Broadcast and is inspired by the implementation on page 124 in this [book](https://link.springer.com/book/10.1007/978-3-642-15260-3).

#### Prerequisites

There are no additional prerequisites needed to enable the broadcast framework functionality. The functionality is compatible with the current version of Gorums. If you are using Gorums for the first time, we refer you to the README file in the root directory.

### Proto file

The broadcast framework provides two proto options:

- broadcastcall: Used as entrypoint for clients.
- broadcast: Used by servers to communicate to each others.

```proto3
import "gorums.proto";

service ReliableBroadcast {
    rpc Broadcast(Message) returns (Message) {
        option (gorums.broadcastcall) = true;
    }
    rpc Deliver(Message) returns (Empty) {
        option (gorums.broadcast) = true;
    }
}

message Message {
    string Data = 1;
}

message Empty {}
```

Notice that the return type of the RPC method `Deliver`. The return type is not used because servers only communicate by broadcasting to each other. The method with broadcastcall does, however, have a return type. This is the type that the servers will reply with when the client invokes `Broadcast`.

### Client

After generating the proto files we can define the client in a file named `client.go`:

```go
type Client struct {
	mgr    *pb.Manager
	config *pb.Configuration
}

func New(addr string, srvAddresses []string, qSize int) *Client {
	mgr := pb.NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	lis, _ := net.Listen("tcp", addr)
	mgr.AddClientServer(lis, lis.Addr())
	config, _ := mgr.NewConfiguration(
		NewQSpec(qSize),
		gorums.WithNodeList(srvAddresses),
	)
	return &Client{
		mgr:    mgr,
		config: config,
	}
}
```

The only addition the broadcast framework brings to how the client is created is the two lines:

```go
lis, _ := net.Listen("tcp", addr)
mgr.AddClientServer(lis, lis.Addr())
```

The first line creates a listener and the second creates a client-side server. This is necessary in order to accept replies from servers not added to the Gorums configuration.

Next we can create a function on the `Client` that can be used to invoke broadcasts on the configuration:

```go
func (c *Client) Broadcast(ctx context.Context, value string) (*pb.Message, error) {
	req := &pb.Message{
		Data: value,
	}
	return c.config.Broadcast(ctx, req)
}
```

To be able to collect responses it is also necessary to create a quorum function. When generating the proto files, Gorums will create a QuorumSpec interface containing all quorum functions. In our example this QuorumSpec is generated:

```go
// QuorumSpec is the interface of quorum functions for ReliableBroadcast.
type QuorumSpec interface {
	gorums.ConfigOption

	BroadcastQF(in *Message, replies []*Message) (*Message, bool)
}
```

We can then proceed to implement the interface by creating a struct named QSpec that contains all the methods in the QuorumSpec:

```go
type QSpec struct {
	quorumSize int
}

func NewQSpec(qSize int) pb.QuorumSpec {
	return &QSpec{
		quorumSize: qSize,
	}
}

func (qs *QSpec) BroadcastQF(in *pb.Message, replies []*pb.Message) (*pb.Message, bool) {
	if len(replies) < qs.quorumSize {
		return nil, false
	}
	return replies[0], true
}
```

This `QSpec` struct is used when the Gorums configuration is created. This can be seen in the code example above when we created the client. Here we provide the `NewQSpec` as one of the arguments to the `mgr.NewConfiguration()` function.

### Server

To create a server that uses the broadcast functionality we can define a file `server.go` containing the server implementation:

```go
type Server struct {
	*pb.Server
	mut       sync.Mutex
	delivered []*pb.Message
	mgr       *pb.Manager
	addr      string
	srvAddrs  []string
}

func New(addr string, srvAddrs []string) *Server {
	lis, _ := net.Listen("tcp", s.addr)
	srv := Server{
		Server:    pb.NewServer(gorums.WithListenAddr(lis.Addr())),
		addr:      addr,
		srvAddrs:  srvAddrs,
		delivered: make([]*pb.Message, 0),
	}
	srv.configureView()
	pb.RegisterReliableBroadcastServer(srv.Server, &srv)
	go srv.Serve(lis)
	return &srv
}
```

The first addition by the broadcast framework when creating the server is that we need to provide the option `gorums.WithListenAddr(lis.Addr())`. This is important because the address of the server is used in the messages sent by the server. Furthermore, we also invoke a function named `configureView()`:

```go
func (srv *Server) configureView() {
	srv.mgr = pb.NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	view, err := srv.mgr.NewConfiguration(gorums.WithNodeList(srv.srvAddrs))
	if err != nil {
		panic(err)
	}
	srv.SetView(view)
}
```

By creating a Gorums configuration and providing it to the generated method `SetView()` we enable server-to-server communication. We use the term `view` when refering to the Gorums configuration on the server side to distinguish it from the configuration created on the client-side.

When we have created the server, we can define the server handlers:

```go
func (s *Server) Broadcast(ctx gorums.ServerCtx, request *pb.Message, broadcast *pb.Broadcast) {
	broadcast.Deliver(request)
}

func (s *Server) Deliver(ctx gorums.ServerCtx, request *pb.Message, broadcast *pb.Broadcast) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if !s.isDelivered(request) {
		s.delivered = append(s.delivered, request)
		broadcast.Deliver(request)
		broadcast.SendToClient(request, nil)
	}
}

func (s *Server) isDelivered(message *pb.Message) bool {
	for _, msg := range s.delivered {
		if msg.Data == message.Data {
			return true
		}
	}
	return false
}
```

The server handler signatures have changed a little, as evident from the code. The broadcast framework removes the return types and introduces a new argument named `broadcast`. This struct is the main interface for interacting with the broadcast framework. Each RPC method in the protofile with the option `gorums.broadcast = true` will be generated on the `broadcast struct`.

The server handler `Broadcast` is quite simple and only contains a single invocation `broadcast.Deliver(request)`. This invocation will broadcast the request to all servers added to the view.

The next server handler `Deliver` first checks whether the request has already been delivered. If not, it broadcasts `Deliver` to the other servers with the request and sends a reply to the client.

The broadcast framework handles issues related to late client messages, duplicated broadcasts, and routing of messages. Hence, this is a complete code example that is correct according to the [description of the algorithm](https://link.springer.com/book/10.1007/978-3-642-15260-3).

## Options

We have implemented a set of options that can be used to configure the broadcasting functionality. These will be presented in short here:

#### Broadcast Server

- `WithShardBuffer(shardBuffer) ServerOption`: Enables the user to specify the buffer size of each shard. A shard stores a map of broadcast requests. A higher buffer size may increase throughput but at the cost of higher memory consumption. The default is 200 broadcast requests.
- `WithSendBuffer(sendBuffer) ServerOption`: Enables the user to specify the buffer size of the communication channels to the broadcast processor. A higher buffer size may increase throughput but at the cost of higher memory consumption. The default is 30 messages.
- `WithBroadcastReqTTL(ttl) ServerOption`: Configures the duration a broadcast request should live on a server, setting the lifetime of a broadcast processor. The default is 5 minutes.

#### Broadcasting

- `WithSubset(srvAddrs) BroadcastOption`: Allows the user to specify a subset of servers to broadcast to. The server addresses given must be a subset of the addresses in the server view.
- `WithoutSelf() BroadcastOption`: Prevents the server from broadcasting to itself.
- `AllowDuplication() BroadcastOption`: Allows the user to broadcast to the same RPC method more than once for a particular broadcast request.

#### Identification

- `WithMachineID(id) ManagerOption`: Enables the user to set a unique ID for the client. This ID will be embedded in broadcast requests sent from the client, making the requests trackable by the whole cluster. A random ID will be generated if not set, which can cause collisions if there are many clients. The ID is bounded between 0 and 4095.
- `WithSrvID(id) ServerOption`: Enables the user to set a unique ID on the broadcast server. This ID is used to generate BroadcastIDs.
- `WithListenAddr(addr) ServerOption`: Sets the IP address of the broadcast server, which will be used in messages sent by the server. The network of the address must be a TCP network name.

#### Connection

- `WithSendRetries(maxRetries) ManagerOption`: Allows the user to specify how many times the node will try to send a message. The message will be dropped if it fails to send more than the specified number of times. Providing `maxRetries = -1` will retry indefinitely.
- `WithConnRetries(maxRetries) ManagerOption`: Allows the user to specify how many times the node will try to reconnect to a node. The default is no limit, but it will follow a backoff strategy.
- `WithClientDialTimeout(timeout) ServerOption`: Enables the user to set a dial timeout for servers when sending replies back to the client in a BroadcastCall. The default is 10 seconds.
- `WithServerGrpcDialOptions(opts) ServerOption`: Enables the user to set gRPC dial options that the Broadcast Router uses when connecting to a client.

#### Logging

- `WithLogger(logger) ManagerOption`: Enables the user to provide a structured logger for the Manager. This will log events regarding the creation of nodes and the transmission of messages.
- `WithSLogger(logger) ServerOption`: Enables the user to set a structured logger for the Server. This will log internal events regarding broadcast requests.

#### Authentication

- `WithAllowList(allowed) ServerOption`: Enables the user to provide a list of (address, publicKey) pairs which will be used to validate messages.
- `the allow list are permitted to send messages to the server, and the server is only allowed to send replies to nodes on the allow list.
- `EnforceAuthentication() ServerOption`: Requires that messages are signed and validated; otherwise, the server will drop them.
- `WithAuthentication() ManagerOption`: Enables digital signatures for messages.

#### Execution Ordering

- `WithOrder(method_1, ..., method_n) ServerOption`: Enables the user to specify the order in which methods should be executed. This option does not order messages but caches messages meant for processing at a later stage. For example, in PBFT, it caches all commit messages if the state is not prepared yet.
- `ProgressTo(method_i) BroadcastOption`: Allows the server to accept messages for the given method or for methods prior in the execution order.
