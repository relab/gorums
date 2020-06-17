# gorums

[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/relab/gorums/raw/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/relab/gorums?status.svg)](https://godoc.org/github.com/relab/gorums)
[![Travis Build Status](https://travis-ci.org/relab/gorums.svg?branch=master)](https://travis-ci.org/relab/gorums)
![golangci-lint](https://github.com/relab/gorums/workflows/golangci-lint/badge.svg)

Gorums [1] is a novel framework for building fault tolerant distributed systems.
Gorums offers a flexible and simple quorum call abstraction, used to communicate
with a set of processes, and to collect and process their responses. Gorums
provides separate abstractions for (a) selecting processes for a quorum call
and (b) processing replies. These abstractions simplify the main control flow
of protocol implementations, especially for quorum-based systems, where only a
subset of the replies to a quorum call need to be processed.

Gorums uses code generation to produce an RPC library that clients can use to
invoke quorum calls. Gorums is a wrapper around the [gRPC](http://www.grpc.io/)
library. Services are defined using the protocol buffers interface definition
language.

## System Requirements

To build and deploy Gorums, you need the following software installed:

* Protobuf compiler (protoc)
* Make
* Ansible (used by benchmark script)

## Examples

The original EPaxos implementation modified to use Gorums can be found
[here](https://github.com/relab/epaxos).

A collection of different algorithms for reconfigurable atomic storage
implemented using Gorums can be found
[here](https://github.com/relab/smartmerge).

## Documentation

* [User guide](doc/userguide.md)
* [Developer guide](doc/devguide.md)

## References

[1] Tormod Erevik Lea, Leander Jehl, and Hein Meling.
    _Towards New Abstractions for Implementing Quorum-based Systems._
    In 37th International Conference on Distributed Computing Systems (ICDCS), Jun 2017.
