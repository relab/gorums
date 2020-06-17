# Benchmarking Gorums

The repostiory includes a program that can be used to benchmark different calltypes and options.
The program is compiled from `cmd/benchmark`, and uses the `benchmark` package.
Using this program, it is possible to run benchmarks on both local and remote servers.

## Usage

To compile the benchmark program, run `make benchmark`

By default, the program runs all of the built in benchmarks on local servers.
The following command line flags can be used to change various parameters of the benchmarks:

```text
Usage of cmd/benchmark/benchmark:
  -benchmarks regexp
      A regexp matching the benchmarks to run. (default '.*')
  -concurrent int
      Number of goroutines that can make calls concurrently. (default 1)
  -config-size int
      Size of the configuration to use. If < 1, all nodes will be used. (default 4)
  -cpuprofile file
      A file to write cpu profile to.
  -max-async int
      Maximum number of async calls that can be in flight at once. (default 1000)
  -memprofile file
      A file to write memory profile to.
  -payload int
      Size of the payload in request and response messages (in bytes).
  -quorum-size int
      Number of replies to wait for before completing a quorum call.
  -remotes list
      A comma separated list of remote addresses to connect to.
  -send-buffer uint
      The size of the send buffer.
  -server address
      Run a benchmark server on given address.
  -server-buffer uint
      The size of the server buffers.
  -server-stats
      Show server statistics separately
  -time duration
      The duration of each benchmark. (default "1s")
  -trace file
      A file to write trace to.
  -warmup duration
      Warmup duration. (default "100ms")
```

By default, the `cmd/benchmark` program starts internal servers to perform benchmarks locally.
To run the benchmarks with remote servers, the `--remotes` flag must be specified.
