# Benchmarking Gorums

The repository includes a program that can be used to benchmark different calltypes and options.
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
To run the benchmarks with remote servers, the `--remotes` flag must be used.

### Remote benchmarks with ansible

In the `scripts/` folder we provide some simple ansible scripts that can be used to run benchmarks on remote servers.
To use the scripts, an [inventory](https://docs.ansible.com/ansible/latest/user_guide/intro_inventory.html) file must be created.
The ansible script expects two groups, "client" and "servers".
Below is an example inventory file:

```ini
[client]
client.example.com

[servers]
server1.example.com
server2.example.com
server3.example.com
```

To copy the benchmark binary to the remote servers, run the `deploy.yml` ansible script as follows

```sh
ansible-playbook -i [your inventory file] deploy.yml
```

(remember to build it using `make benchmark` first).

There is a simple script `benchmark.sh` that runs the appropriate ansible-playbook command, and parses the output.
To use this, you must first `cd` into the `scripts/` directory, and then run

```sh
./benchmark.sh [your inventory file] [arguments to benchmark]
```

For example:

```sh
./benchmark.sh ./hosts --benchmarks 'QC'
```
