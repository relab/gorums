# Tests that should be run each time
RUNTESTS := ordering metadata tls unresponsive

.PHONY: all runtests qf order metadata tls unresponsive

all: $(RUNTESTS)

qf: qf/qf.pb.go qf/qf_gorums.pb.go

ordering: ordering/order.pb.go ordering/order_gorums.pb.go

metadata: metadata/metadata.pb.go metadata/metadata_gorums.pb.go

tls: tls/tls.pb.go tls/tls_gorums.pb.go

unresponsive: unresponsive/unresponsive.pb.go unresponsive/unresponsive_gorums.pb.go

%.pb.go : %.proto
	@protoc -I=..:. --go_out=paths=source_relative:. $<

%_grpc.pb.go : %.proto
	@protoc -I=..:. --go-grpc_out=paths=source_relative:. $<

%_gorums.pb.go : %.proto
	@protoc -I=..:. --gorums_out=paths=source_relative:. $<

clean:
	rm metadata/metadata.pb.go metadata/metadata_gorums.pb.go
	rm ordering/order.pb.go ordering/order_gorums.pb.go
	rm tls/tls.pb.go tls/tls_gorums.pb.go
	rm qf/qf.pb.go qf/qf_grpc.pb.go qf/qf_gorums.pb.go
