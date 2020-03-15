
.PHONY: basic
basic:
	@protoc \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		basic/basic.proto

.PHONY: qc
qc:
	@protoc -I../../..:. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=trace=true,paths=source_relative:. \
		quorumcall/quorumcall.proto

.PHONY: clean
clean:
	@rm -f quorumcall/*.go
	@rm -f basic/*.go
