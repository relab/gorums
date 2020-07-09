.PHONY: all qf order message_encoding metadata tls reconnect

all: qf order message_encoding metadata tls reconnect

qf: qf/qf.pb.go qf/qf_grpc.pb.go qf/qf_gorums.pb.go

order: ordering/order.pb.go ordering/order_gorums.pb.go

message_encoding: message_encoding/message_encoding.pb.go message_encoding/message_encoding_gorums.pb.go

metadata: metadata/metadata.pb.go metadata/metadata_gorums.pb.go

tls: tls/tls.pb.go tls/tls_gorums.pb.go

reconnect: reconnect/reconnect.pb.go reconnect/reconnect_gorums.pb.go

%.pb.go : %.proto
	@protoc -I=..:. --go_out=paths=source_relative:. $<

%_grpc.pb.go : %.proto
	@protoc -I=..:. --go-grpc_out=paths=source_relative:. $<

%_gorums.pb.go : %.proto
	@protoc -I=..:. --gorums_out=paths=source_relative,trace=true:. $<
