# Gorums examples

This folder contains examples of services implemented with Gorums.
If you want a detailed walkthrough of how to get started with Gorums, read the [user guide](../doc/user-guide.md).

## Interactive Storage service

The `storage` example implements a key-value storage service with an interactive Read-Eval-Print Loop (REPL).
The client and server share one binary.

### Running the Storage Example

Run these commands from the repository root.
To start four local servers and the REPL:

```shell
go run ./examples/storage
```

To connect to existing servers, pass their addresses separated by commas:

```shell
go run ./examples/storage -addrs localhost:8080,localhost:8081,localhost:8082
```

To enable server-side interceptors, pass a comma-separated list of names:

```shell
go run ./examples/storage -interceptors logging,nofoo
```

Available interceptors: `logging`, `nofoo`, `metadata`, `delayed`.

To build a binary with [just](https://github.com/casey/just):

```shell
just storage
./examples/storage/storage
```

Alternatively, build it with Go:

```shell
go build -o ./examples/storage/storage ./examples/storage
```

To recompile the proto files:

```shell
just examples-proto
```

### REPL Commands

Once the REPL starts, the following commands are available:

| Command                            | Description                                        |
| ---------------------------------- | -------------------------------------------------- |
| `help`                             | Print the help message                             |
| `nodes`                            | List available nodes and their addresses           |
| `exit`                             | Exit the REPL                                      |
| `tick [count]`                     | Publish a counter each second; Ctrl-C stops it     |
| `rpc <node> read <key>`            | Read from a single node via ordered RPC            |
| `rpc <node> write <key> <value>`   | Write to a single node via ordered RPC             |
| `ucast <node> <key> <value>`       | One-way unicast write to a single node             |
| `mcast <key> <value>`              | One-way multicast write to all nodes               |
| `qc read <key>`                    | Read quorum call on all nodes                      |
| `qc write <key> <value>`           | Write quorum call on all nodes                     |
| `qc cread <key>`                   | Correctable read quorum call (streams updates)     |
| `qc nread <key>`                   | Nested read quorum call (server fans out to peers) |
| `qc nwrite <key> <value>`          | Nested write quorum call via multicast             |
| `cfg <nodes> read <key>`           | Read quorum call on a node sub-configuration       |
| `cfg <nodes> write <key> <value>`  | Write quorum call on a node sub-configuration      |
| `cfg <nodes> cread <key>`          | Correctable read on a sub-configuration            |
| `cfg <nodes> nread <key>`          | Nested read on a sub-configuration                 |
| `cfg <nodes> nwrite <key> <value>` | Nested write via multicast on a sub-configuration  |

The `<node>` argument is a zero-based index into the node list shown by `nodes`.
The `<nodes>` argument for `cfg` is a range (`1:3` selects nodes 1 and 2) or a comma-separated list (`0,2` selects nodes 0 and 2).
Values containing spaces must be quoted: `qc write greeting 'hello world'`.

The `nread` and `nwrite` commands make nested calls to server peers through `ctx.PeerConfig()`.
Callbacks through `ctx.ConnectedClients()` require the client to register method handlers with a back channel; the REPL client provides only outgoing calls.
See [Nested Quorum Calls](../doc/user-guide.md#nested-quorum-calls-with-servercontextpeerconfig) and [Reverse Direction Calls](../doc/user-guide.md#reverse-direction-calls-with-servercontextconnectedclients) in the user guide.

### REPL Example Session

```text
> nodes
Nodes:
0: 127.0.0.1:54321
1: 127.0.0.1:54322
2: 127.0.0.1:54323
3: 127.0.0.1:54324

> qc write greeting hello
Write OK

> qc read greeting
greeting = hello

> rpc 0 read greeting
greeting = hello

> ucast 1 greeting world
Unicast OK

> mcast greeting gorums
Multicast OK

> cfg 0,1 read greeting
greeting = gorums

> qc cread greeting
greeting = gorums (level 1)
greeting = gorums (level 2)
Correctable read finished

> qc nread greeting
greeting = gorums

> exit
```

## Periodic state dissemination

The `tick` command publishes a counter with `Send()` every second.
Use `tick 5` for five updates, or `tick` to run until interrupted; Ctrl-C returns to the prompt in either case.
Read the value with `qc read counter`.

Each run starts at one and timestamps its updates, so servers keep the latest value and later updates repair missed writes.
The client reports queue admissions and per-node admission errors while Gorums transmits accepted updates in the background.
See [disseminate.go](storage/disseminate.go) and [Send-Timing Model](../doc/user-guide.md#send-timing-model) for details.
