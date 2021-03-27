# Gorums examples

This folder contains examples of services implemented with Gorums.
If you want a detailed walkthrough of how to get started with Gorums, read the [user guide](../doc/user-guide.md).

## Interactive Storage service

The `storage` example implements a simple key-value storage service.
The client features an interactive command line interface that allows you to send RPCs and quorum calls to different configurations of servers, called nodes.
Both the client and the server are included in the same binary, and four local servers will be started automatically.

To install:

```shell
make tools
make
```

Optionally:

```shell
go build -o ./storage ./storage
```

To run:

```shell
./storage/storage
```

To recompile the proto files:

```shell
make -B
```
