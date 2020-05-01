all: grpc qc order

.PHONY: grpc
grpc:
	@protoc \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		grpc/grpc.proto

.PHONY: qc
qc:
	@protoc -I../../..:. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=trace=true,paths=source_relative:. \
		quorumcall/quorumcall.proto

.PHONY: order
order:
	@protoc -I.:../../.. \
		--go_out=paths=source_relative:. \
		--gorums_out=trace=true,paths=source_relative:. \
		ordering/order.proto

.PHONY: clean
clean:
	@rm -f quorumcall/*.go
	@rm -f grpc/*.go
