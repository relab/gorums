PROTO_SRC := $(wildcard *.proto) $(wildcard **/*.proto)
PROTO_GO := $(PROTO_SRC:%.proto=%.pb.go)
GRPC := benchmark_grpc.pb.go
GORUMS := benchmark_gorums.pb.go

PROTOC_FLAGS := -I=../.:.

.PHONY: all benchmain server

all: $(PROTO_GO) $(GRPC) $(GORUMS) benchmain server

$(PROTO_GO): %.pb.go : %.proto
	@protoc --go_out=paths=source_relative:. $(PROTOC_FLAGS) $^

$(GRPC): %_grpc.pb.go : %.proto
	@protoc --go-grpc_out=paths=source_relative:. $(PROTOC_FLAGS) $^

$(GORUMS): %_gorums.pb.go : %.proto
	@protoc --gorums_out=paths=source_relative,trace=true:. $(PROTOC_FLAGS) $^

benchmain:
	@go build -o benchmain/benchmain ./benchmain

server:
	@go build -o server/server ./server
