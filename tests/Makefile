# Tests that should be run each time
RUNTESTS := ordering encoding metadata tls reconnect

.PHONY: all runtests qf order encoding metadata tls reconnect

all: $(RUNTESTS)

runtests: $(RUNTESTS)
	@for package in $^; do \
		go test -v ./$$package; \
	done

qf: qf/qf.pb.go qf/qf_grpc.pb.go qf/qf_gorums.pb.go

ordering: ordering/order.pb.go ordering/order_gorums.pb.go

encoding: encoding/encoding.pb.go encoding/encoding_gorums.pb.go

metadata: metadata/metadata.pb.go metadata/metadata_gorums.pb.go

tls: tls/tls.pb.go tls/tls_gorums.pb.go

reconnect: reconnect/reconnect.pb.go reconnect/reconnect_gorums.pb.go

%.pb.go : %.proto
	@protoc -I=..:. --go_out=paths=source_relative:. $<

%_grpc.pb.go : %.proto
	@protoc -I=..:. --go-grpc_out=paths=source_relative:. $<

%_gorums.pb.go : %.proto
	@protoc -I=..:. --gorums_out=paths=source_relative,trace=true:. $<
