# gorums

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

### Examples

The original EPaxos implementation modified to use Gorums can be found
[here](https://github.com/relab/epaxos).

A collection of different algorithms for reconfigurable atomic storage
implemented using Gorums can be found
[here](https://github.com/relab/smartmerge).

### Outdated documentation

* [Student/user guide](doc/userguide.md)
* [Developer guide](doc/devguide.md)

### Adding a new extension option (TODO Move this to devguide)

1. Add your extension option to `gorums.proto`. We currently only have method options.
2. Run `make gorumsprotoopts` to regenerate the `gorums.pb.go` file. (TODO we could probably avoid using a make file for this and instead do `go generate`)
3. Add a check function, such as `hasPerNodeArgExtension()`, for your option in `plugins/gorums/ext.go`.
4. Update the `plugins/gorums/gorums.go` as follows 
   a. add the option `PerNodeArg` bool to the `serviceMethod` struct.
   b. add the option to initialize of the `serivceMethod` struct in the `verifyExtensionsAndCreate` function, like this: `PerNodeArg:        hasPerNodeArgExtension(method),`
   c. update the logic in the `isQuorumCallVariant` function if necessary.
   d. update the error handling logic in `verifyExtensionsAndCreate`.

5. Update the template files (`.tmpl` in `dev` folder) related to your option. This is were your on your own.

### References

[1] Tormod E. Lea, Leander Jehl, and Hein Meling. _Gorums: New Abstractions for
    Implementing Quorum-based Systems._ In submission.
