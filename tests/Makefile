all: qf order

.PHONY: qf
qf:
	@protoc -I../:. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=trace=true,paths=source_relative:. \
		qf/qf.proto

.PHONY: order
order:
	@protoc -I../:. \
		--go_out=paths=source_relative:. \
		--gorums_out=trace=true,paths=source_relative:. \
		ordering/order.proto

.PHONY: clean
clean:
	@rm -f quorumcall/*.go
	@rm -f grpc/*.go
	@rm -f ordering/*.pb.go
