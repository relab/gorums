### Available options and their meaning

Below is shown an example service definition from a proto file. Gorums currently supports the following call types 

| Call type     | Gorums option      | Description                                 |
|---------------|--------------------|---------------------------------------------|
| Regular gRPC  | no option          | If no option is specified no Gorums functionality is applied to the RPC method. |
| Quorum Call   | `gorums.qc`        | This option will generate a call function that returns once a quorum of replies have been collected by the Gorums runtime. This is determined by a quorum function explained elsewhere. |
| Future        | `gorums.qc_future` | Asynchronously call the function. |
| Correctable   | `gorums.correctable` | TBD - TODO Rename this call type? |
| Correctable Prelim  | `gorums.correctable_pr` | TBD - TODO Rename this call type? |
| Multicast     | `gorums.multicast` | One way multicast. No replies are collected. |

Each call type may in addition specify some advanced options:

| Name          | Gorums option      | Description                                 |
|---------------|--------------------|---------------------------------------------|
| Custom return type | `gorums.custom_return_type` | This option specified an custom return type that the quorum function can populate with additional information for the user application, typically based on the type specified in the RPC definition. |
| Per node arguments | `gorums.per_node_arg`       | This option tells Gorums to generate an RPC method that takes a special function that computes the argument to be sent to the different servers in the configuration. |

P.S. Multicast and some other call types may not support the advanced options yet. The compiler should complain if an unsupported combination is specified.


```proto
service Storage {
	// ReadNoQC is a plain gRPC call.
	rpc ReadNoQC(ReadRequest) returns (State) {}

	// Read is a synchronous quorum call.
	rpc Read(ReadRequest) returns (State) {
		option (gorums.qc) = true;
	}

	// ReadFuture is an asynchronous quorum call that 
	// returns a future object for retrieving results.
	rpc ReadFuture(ReadRequest) returns (State) {
		option (gorums.qc_future) = true;
	}

	// ReadCustomReturn is a synchronous quorum call with a custom return type
	rpc ReadCustomReturn(ReadRequest) returns (State) {
		option (gorums.qc) 			= true;
		option (gorums.custom_return_type) 	= "MyState";
	}

	// ReadCorrectable is an asynchronous correctable quorum call that 
	// returns a correctable object for retrieving results.
	rpc ReadCorrectable(ReadRequest) returns (State) {
		option (gorums.correctable) = true;
	}

	// ReadPrelim is an asynchronous correctable quorum call that 
	// returns a correctable object for retrieving results.
	rpc ReadPrelim(ReadRequest) returns (stream State) {
		option (gorums.correctable_pr) = true;
	}

	// Write is a synchronous quorum call.
	// The request argument (State) is passed to the associated
	// quorum function, WriteQF, for this method.
	rpc Write(State) returns (WriteResponse) {
		option (gorums.qc)		= true;
		option (gorums.qf_with_req)	= true;
	}

	// WriteFuture is an asynchronous quorum call that 
	// returns a future object for retrieving results.
	// The request argument (State) is passed to the associated
	// quorum function, WriteFutureQF, for this method.
	rpc WriteFuture(State) returns (WriteResponse) {
		option (gorums.qc_future)	= true;
		option (gorums.qf_with_req)	= true;
	}

	// WriteAsync is an asynchronous multicast to all nodes in a configuration.
	// No replies are collected.
	rpc WriteAsync(stream State) returns (Empty) {
		option (gorums.multicast) = true;
	}

	// WritePerNode is a synchronous quorum call, where,
	// for each node, a provided function is called to determine
	// the argument to be sent to that node.
	rpc WritePerNode(State) returns (WriteResponse) {
		option (gorums.qc)		= true;
		option (gorums.per_node_arg) 	= true;
	}
}
```

### Adding a new extension option

1. Add your extension option to `gorums.proto`. We currently only have method options.
2. Run `make gorumsprotoopts` to regenerate the `gorums.pb.go` file. (TODO we could probably avoid using a make file for this and instead do `go generate`)
3. Add a check function, such as `hasPerNodeArgExtension()`, for your option in `plugins/gorums/ext.go`.
4. Update the `plugins/gorums/gorums.go` as follows 
   a. add the option `PerNodeArg` bool to the `serviceMethod` struct.
   b. add the option to initialize of the `serivceMethod` struct in the `verifyExtensionsAndCreate` function, like this: `PerNodeArg:        hasPerNodeArgExtension(method),`
   c. update the logic in the `isQuorumCallVariant` function if necessary.
   d. update the error handling logic in `verifyExtensionsAndCreate`.
5. Update the template files (`.tmpl` in `dev` folder) related to your option. This is were your on your own.
6. To regenerate the gorums plugin, you need to run `make dev` so that new proto files will understand your option.
7. Update the `dev/storage.proto` file with your new option (probably on a new method).
8. To use the new option in the `dev/storage.proto` file, you need to run `make devproto`.
