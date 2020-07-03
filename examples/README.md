# Gorums examples

This folder contains different examples of services implemented with Gorums.
If you want detailed walkthrough of how to get started with Gorums, read the [user guide](../doc/userguide.md).

## Prerequisites

Requires Go 1.13 or later and you must have `$GOPATH/bin` in your `$PATH`:

<https://github.com/golang/go/wiki/SettingGOPATH>

## Interactive Storage service

The `storage` example implements a simple key-value storage service.
The client features an interactive command line interface that allows you to send RPCs and quorum calls to different configurations of servers, called Nodes.

Install:

`go get -u github.com/relab/gorums/examples/storage`

Run:

`storage`

## (Optional) Compile examples with Make

Run `make` in this folder to compile all examples.
Requires a recent `protoc` version 3.

If you need to recompile the proto files, you can run `make -B`.
