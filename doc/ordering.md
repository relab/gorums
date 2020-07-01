# Message Ordering in Gorums

## The Problem

Many of the use cases for a framework like Gorums depend on messages arriving in the correct order.
Unfortunately, gRPC does not guarantee that unary RPCs invoked in order, will be received by the server in the same order.
That is, according to the [gRPC docs](https://grpc.io/docs/what-is-grpc/core-concepts/):
"_gRPC guarantees message ordering within an individual RPC call._"
Further, as explained [here](https://github.com/grpc/grpc/issues/10853#issuecomment-297478862):
"_Separate requests are independent of each other, and there is no guarantee that they will be handled in any particular order. If you want to preserve the order of a set of messages between a single client and a single server, I would recommend using a streaming call. Messages within a single stream are guaranteed to be received in the order in which they are sent._"
This was a source of [problems](https://github.com/relab/gorums/issues/16) in early versions of Gorums.

## The Quorum Call and Message Ordering

Gorums supports two implementations of the quorum call.
The current default, enabled with the `gorums.quorumcall` option, is based on gRPC's unary RPCs, and does not preserve ordering of messages as described above.

In addition, Gorums includes another quorum call implementation that can be activated by using the `gorums.ordered` option.
The ordered quorum call implementation relies on gRPC streams, which does
[preserve the order of messages](https://grpc.io/docs/what-is-grpc/core-concepts/), instead of unary RPCs.

The ordered version is intended to become the default at some point.

## Implementation of Message Ordering

### Client-side

Under the hood, Gorums sends messages on a single gRPC stream for each Node.
Requests are given a unique message ID to identify requests and responses that belong together.
The messages are then sent along with some other metadata required by Gorums.
The sending and receiving of messages on the client side is handled by two goroutines for each *node stream*.
When you make a quorum call, the quorum call method passes the request message to each node's *sending goroutine*.
The quorum call method also creates a channel to receive the response messages.
When a response message is received by a node's *receiving goroutine*, it will be passed back to the quorum call method on the channel that was created.

### Server-side

The server side functions similarly.
There are two goroutines, one responsible for sending and one responsible for receiving messages.
Upon receiving a request, the receiving goroutine *synchronously* executes the handler for that request.
Before the handler is called, Gorums creates a channel to receive the response message.
When Gorums receives a message on this channel, it gets tagged with the same message ID as the request, and is then sent back to the client.

The use of a channel to return messages from the RPC handler makes the handlers a lot more flexible than a function that simply returns a result:

* Since the handler functions are executed synchronously by the receiving goroutine, we can ensure that requests are processed in the correct order.
* You can still do concurrent processing of requests if you start a goroutine from the RPC handler.

### RPC API Differences from gRPC

```go
// the "old" gRPC-style server API
type Server interface {
  // Handler takes a request and returns a response.
  // Runs synchronously.
  RPC(*Request) *Response
}

// the "new" Gorums server API
type Server interface {
  // Handler takes a request and a channel to send the response on.
  // Runs synchronously, but may spawn goroutines and return early.
  // This allows the server to start processing the next request.
  // Goroutines can then asynchronously send response on channel.
  RPC(*Request, chan<- *Response)
}
```

The RPC API of Gorums was a lot closer to gRPC in older versions.
However, as we worked on implementing message ordering in Gorums, we recognized that we needed a more flexible API in order to offer good performance.
Specifically, we decided to use channels instead of the `return` statement when passing response messages back from server handlers.
In order to ensure message ordering, the server has to receive and process messages synchronously (i.e. one at a time).
Thus, we want to ensure that the time it takes to process a single request is as short as possible.

However, the application may only need to worry about message ordering up to a certain point.
For example, let's consider an application that needs to send requests to servers in-order, but the order of the responses don't matter.
The handler for this application needs to put the request into a queue, and then wait for the request to be processed before returning a response.
With a gRPC-style RPC handler API (a function takes the request and returns the response),
it is not possible to start any goroutines without also waiting for them to return.
In other words, the handler must **block** the server until the response is ready.
However, with our channel-based API, the handler can simply add the request to the queue,
start a goroutine to wait for the response and pass it to the channel, and then return.
While the goroutine stared by the handler is waiting, another request can be processed by the server.
This the penalty for running server handlers synchronously is reduced, and ordering can still be preserved.

## Notes

There are some important things to note about message ordering:

* While Gorums does preserve the ordering of messages, it is your responsibility to preserve the order at both endpoints.
For example, if you need to preserve message ordering, you cannot start quorum calls in separate goroutines, as the scheduling of goroutines is not deterministic.
If you need message ordering but want to process replies to multiple quorum calls concurrently, use the `async` option.

* Gorums preserves the ordering of messages end-to-end.
But if you need the server to return replies in the same order as the client sent them, you need to ensure that the server-side handler also preserves ordering.
