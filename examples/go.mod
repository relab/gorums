module github.com/relab/gorums/examples

go 1.25.4

require (
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/relab/gorums v0.9.0
	golang.org/x/term v0.36.0
	google.golang.org/grpc v1.76.0
	google.golang.org/protobuf v1.36.10
)

require (
	golang.org/x/net v0.46.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251103181224-f26f9409b101 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
)

replace github.com/relab/gorums => ../

tool (
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
