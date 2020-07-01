# Message ordering in Gorums

Many of the use cases for a framework like Gorums depend on messages arriving in the correct order.
Due to this need, Gorums has implemented features that make it easy to preserve the ordering of messages.
Currently, Gorums supports two implementations of the quorum call.
The current default is based on gRPC's unary RPCs.
Unfortunately, gRPC does not guarantee that unary RPCs invoked in order will be received by the server in the same order.
Because of this, Gorums includes another quorum call implementation that can be activated by using the `gorums.ordered` flag.
The ordered quorum call implementation relies on gRPC streams, which
[preserve the order of messages](https://grpc.io/docs/what-is-grpc/core-concepts/), instead of unary RPCs.
The ordered version is intended to become the default at some point.

## Implementation

Under the hood, Gorums sends messages on a single gRPC stream for each Node.
Requests are given a unique message ID to identify requests and responses that belong together.
The messages are then sent along with some other metadata required by gorums.
The sending and receiving of messages on the client side is handled by two goroutines for each *node stream*.
When you make a quorum call, the quorum call method passes the request message to each node's *sending goroutine*.
The quorum call method also creates a channel to receive the response messages.
When a response message is received by a node's *receiving goroutine*, it will be passed back to the quorum call method on the channel that was created.

The server side function similarly.
Two goroutines are responsible for sending / receiving messages.
Upon receiving a request, the receiving goroutine *synchronously* executes the handler for that request.
Before the handler is called, Gorums creates a channel to receive the response message.
When Gorums receives a message on this channel, it gets tagged with the same message ID as the request, and then gets sent back to the client.

The use of channels to return messages from the RPC handler makes the handlers a lot more flexible than a function that simply returns a result:

* Since the handler functions are executed synchronously by the receiving goroutine, we can ensure that requests are processed in the correct order.
* You can still do concurrent processing of requests if you start a goroutine from the RPC handler.

## Notes

There are some important things to note about message ordering:

* While Gorums does preserve the ordering of messages, it is your responsibility to preserve the order at both endpoints.
For example, if you need to preserve message ordering, you cannot start quorum calls in separate goroutines, as the scheduling of goroutines is not deterministic.
If you need message ordering but want to process replies to multiple quorum calls concurrently, use the `async` option.

* Gorums preserves the ordering of messages end-to-end.
But if you need the server to return replies in the same order as the client sent them, you need to ensure that the server-side handler also preserves ordering.
