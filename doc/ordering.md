# Message ordering in Gorums

Many of the use cases for a framework like Gorums depends on messages arriving in the correct order.
Due to this need, Gorums has gained features that help preserve the ordering of messages.
Currently, Gorums supports two implementations of the quorum call.
The current default is based on gRPC's unary RPCs.
Unfortunately, gRPC does not guarantee that unary RPCs invoked in order will be received by the server in the same order.
Because of this, Gorums includes another quorum call implementation that can be activated by using the `gorums.ordered` flag.
The ordered quorum call implementation relies on gRPC streams, which
[preserve the order of messages](https://grpc.io/docs/what-is-grpc/core-concepts/), instead of unary RPCs.
The ordered version is intended to become the default at some point.

There are some important things to note about message ordering:

* While Gorums does preserve the ordering of messages, it is your responsibility to preserve the order at both endpoints.
For example, if you need to preserve message ordering, you cannot start quorum calls in separate goroutines, as the scheduling of goroutines is not deterministic.
If you need message ordering but want to process replies to multiple quorum calls concurrently, use the `async` option.

* Gorums preserves the ordering of messages end-to-end.
But if you need the server to return replies in the same order as the client sent them, you need to ensure that the server-side handler also preserves ordering.
