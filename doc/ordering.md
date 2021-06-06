# Message Ordering in Gorums

## The Problem

Many of the use cases for a framework like Gorums depend on messages arriving in the correct order.
Unfortunately, gRPC does not guarantee that unary RPCs invoked in order, will be received by the server in the same order.
That is, according to the [gRPC docs](https://grpc.io/docs/what-is-grpc/core-concepts/):
"_gRPC guarantees message ordering within an individual RPC call._"
Further, as explained [here](https://github.com/grpc/grpc/issues/10853#issuecomment-297478862):
"_Separate requests are independent of each other, and there is no guarantee that they will be handled in any particular order. If you want to preserve the order of a set of messages between a single client and a single server, I would recommend using a streaming call. Messages within a single stream are guaranteed to be received in the order in which they are sent._"
This was a source of [problems](https://github.com/relab/gorums/issues/16) in early versions of Gorums.

## Implementation of Message Ordering

Gorums now implements ordered RPCs between client and server, in addition to ordered quorum calls.
This implementation relies on gRPC streams to [preserve the order of messages](https://grpc.io/docs/what-is-grpc/core-concepts/) instead of unary RPCs.

### Client-side

Under the hood, Gorums sends messages on a single gRPC stream for each node.
Request messages are assigned unique message IDs to associate requests and responses that belong together.
Messages are then sent along with some other metadata required by Gorums.
Two goroutines for each *node stream* handle the sending and receiving of messages on the client-side.
When invoking a quorum call, a request message is passed to each node's *sending goroutine*.
The quorum call method also creates a channel for receiving response messages.
When a node's receiving goroutine receives a response message, it will be passed back to the quorum call method via its associated channel.

### Server-side

The server-side works similarly to the client-side.
There are two goroutines, one responsible for sending and one responsible for receiving messages.
Upon receiving a request, the receiving goroutine *synchronously* executes the handler for that request.
Before the handler is called, Gorums creates a channel to receive the response message.
When Gorums receives a message on this channel, it gets tagged with the same message ID as the request.

The use of a function to return messages from the RPC handler makes the handlers more flexible than handlers that simply return a result:

* RPC handler functions are executed synchronously by the receiving goroutine, ensuring in-order processing of requests.
* Concurrent processing of requests is still possible since the RPC handler can start a goroutine.

### RPC API Differences from gRPC

```go
// the unary gRPC-style server API
type Server interface {
  // Runs in one of multiple worker goroutines,
  // and requests may be handled in a different order.
  RPC(context.Context, *Request) (*Response, error)
}

// the Gorums server API
type Server interface {
  // Handler receives a special server context object.
  // Runs in its own goroutine.
  // Server waits until the handler returns
  // or until the handler calls Release() on the context object.
  RPC(gorums.ServerCtx, *Request) (*Response, error)
}
```

Our server handlers are executed synchronously by default, in the order that requests are received in.
This allows us to support use cases where message ordering is important.
However, the application may only need to worry about message ordering up to a certain point.
For example, consider an application that needs to send requests to servers in-order, but can receive responses in any order.
For this application, the handler may place the request in a queue, wait for it to be processed before returning a response.
With our API, the handler can simply add the request to the queue, call `Release()` on the server context,
and then return the response once it is ready.
Hence, the penalty for running server handlers synchronously is reduced while still preserving ordering.
Below is an example of how such a handler could be written:

```go
func (s *testSrv) AsyncHandler(ctx gorums.ServerCtx, req *Request) (resp *Response, err error) {
  // do synchronous work
  response := &Response{
    InOrder: s.isInOrder(req.GetNum()),
  }

  // allow the server to start processing the next request.
  ctx.Release()

  // this code will run concurrently with other handlers
  // perform slow / async work here
  time.Sleep(10 * time.Millisecond)
  // at some point later, the response passed back to Gorums through the `ret` function,
  // and gets sent back to the client.
  return response, nil
}
```

## How to Preserve Message Ordering

While Gorums preserve message ordering end-to-end, this guarantee only holds as long as the order is preserved at both endpoints.
Hence, to preserve message ordering, the following rules must be adhered to:

* (Client-side) Quorum calls cannot be started in separate goroutines, as the scheduling of goroutines is non-deterministic.

* (Client-side) To process replies from different quorum calls concurrently, use the `async` option.

* (Server-side) If the server must return replies in the same order as the client sent them, the server-side handler must also preserve ordering.
