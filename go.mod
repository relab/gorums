module github.com/relab/gorums

go 1.25.4

require (
	github.com/google/go-cmp v0.7.0
	go.uber.org/goleak v1.3.0
	golang.org/x/sync v0.18.0
	golang.org/x/tools v0.38.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251103181224-f26f9409b101
	google.golang.org/grpc v1.76.0
	google.golang.org/protobuf v1.36.10
)

require (
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/net v0.46.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
)

tool (
	golang.org/x/tools/cmd/stress
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
