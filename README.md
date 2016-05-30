# gorums

Gorums [1] is a framework for simplifying the design and implementation of
fault-tolerant quorum-based protocols. Gorums allows to group replicas into one
or more _configurations_. A configuration also holds information on how many
replicas are necessary to form a quorum. Gorums enables programmers to invoke
remote procedure calls (RPCs) on the replicas in a configuration and wait for
responses from a quorum. We call this a quorum remote procedure call (QRPC).

Gorums uses code generation to produce an RPC library that clients can use to
invoke QRPCs. Gorums is a wrapper around the [gRPC](http://www.grpc.io/)
library. Services are defined using the protocol buffers interface definition
language.

### Examples

A collection of different algorithms for reconfigurable atomic storage
implemented using Gorums can be found
[here](https://github.com/relab/smartmerge).

### Documentation

* [Student/user guide](https://github.com/relab/gorums-dev/blob/master/doc/userguide.md)
* [Developer guide](https://github.com/relab/gorums-dev/blob/master/doc/devguide.md)

### References

[1] Tormod E. Lea, Leander Jehl, and Hein Meling. Gorums: _A Framework for
    Implementing Reconfigurable Quorum-based Systems._ In submission.
