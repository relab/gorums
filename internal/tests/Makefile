# Tests that should be run each time
TESTS := qf ordering metadata tls unresponsive oneway config correctable

.PHONY: all clean qf ordering metadata tls unresponsive oneway config correctable

all: $(TESTS)

qf: qf/qf.pb.go qf/qf_gorums.pb.go

ordering: ordering/order.pb.go ordering/order_gorums.pb.go

metadata: metadata/metadata.pb.go metadata/metadata_gorums.pb.go

tls: tls/tls.pb.go tls/tls_gorums.pb.go

unresponsive: unresponsive/unresponsive.pb.go unresponsive/unresponsive_gorums.pb.go

oneway: oneway/oneway.pb.go oneway/oneway_gorums.pb.go

config: config/config.pb.go config/config_gorums.pb.go

correctable: correctable/correctable.pb.go correctable/correctable_gorums.pb.go

%.pb.go : %.proto
	@protoc -I=../..:. --go_out=paths=source_relative:. --go_opt=default_api_level=API_OPAQUE $<

%_grpc.pb.go : %.proto
	@protoc -I=../..:. --go-grpc_out=paths=source_relative:. $<

%_gorums.pb.go : %.proto
	@protoc -I=../..:. --gorums_out=paths=source_relative:. $<

clean:
	find . -name '*.pb.go' -exec rm {} \;
