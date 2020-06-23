.PHONY: all qf order message_encoding

all: qf order message_encoding

qf: qf/qf.pb.go qf/qf_grpc.pb.go qf/qf_gorums.pb.go

order: ordering/order.pb.go ordering/order_gorums.pb.go

message_encoding: message_encoding/message_encoding.pb.go message_encoding/message_encoding_gorums.pb.go

%.pb.go : %.proto
	@protoc -I=..:. --go_out=paths=source_relative:. $<

%_grpc.pb.go : %.proto
	@protoc -I=..:. --go-grpc_out=paths=source_relative:. $<

%_gorums.pb.go : %.proto
	@protoc -I=..:. --gorums_out=paths=source_relative,trace=true:. $<
