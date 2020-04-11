all: grpc qc strictordering

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

.PHONY: strictordering
strictordering:
	@protoc -I../../..:. \
		--go_out=paths=source_relative:. \
		--gorums_out=trace=true,paths=source_relative:. \
		strictordering/strictordering.proto

.PHONY: clean
clean:
	@rm -f quorumcall/*.go
	@rm -f grpc/*.go
