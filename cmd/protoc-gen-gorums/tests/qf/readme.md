# Benchmarks for quorum functions with and without the request object

The question we want to answer in this document is:
_Will Gorums be faster if we generate quorum functions without the `request` parameter, when the `request` is not needed by the quorum function?_

Below are three runs of the benchmark for three variants of using the quorum function.

- The `UseReqQF` method actually receive and use the `request` object passed to the quorum function. If your quorum function needs the request object, you anyway have to use this approach.
- The `IgnoreReqQF` method has the same function signature as `UseReqQF`, but ignores the `request` object by using a `_` in the implementation. This would be the recommended approach if we make the quorum function signature always require the request object.
- The `WithoutReqQF` method has a function signature without the `request` object, and as such it should be the fastest, since it does not need to pass the request object on the stack at all.

All three benchmark methods has the same number of allocations, and so we exclude those details from the results below.

The conclusion from the benchmarks below is that `UseReqQF` is always slower, which is to be expected. However, from three runs of the benchmark on the same machine (a mid-2013 MacPro), it is not possible to observe any relevant difference between `WithoutReqQF` and `IgnoreReqQF`. In conclusion, it does not appear to be any value keeping the `qf_with_req` option.

## Running the benchmarks

First compile the qf proto file.

```sh
cd cmd/protoc-gen-gorums/tests
make qf
```

To plot graphs based on the benchmarks, you will need benchgraph:

```sh
go get -v github.com/AntonioSun/benchgraph
```

To run the benchmarks:

```sh
cd qf
go test -bench=BenchmarkQF | benchgraph -title "Quorum Function Evaluation"
```

To view plots for three separate runs:

- [Run 1](plots/benchgraph-run1.html)
- [Run 2](plots/benchgraph-run2.html)
- [Run 3](plots/benchgraph-run3.html)
