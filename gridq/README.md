# gridq

gorums grid quorum example

## Running localhost example 

#### Start servers

```shell
cd cmd/gqserver
./start3x3.sh
```

#### Start a client

```shell
cd cmd/gqclient
go build
./gqclient -predef=3x3
```

## Quorum function benchmarks

```make bench```
