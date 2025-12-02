module github.com/relab/gorums/examples

go 1.25.4

require (
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/relab/gorums v0.10.0
	golang.org/x/term v0.37.0
	google.golang.org/grpc v1.76.0
	google.golang.org/protobuf v1.36.10
)

require (
	go.uber.org/goleak v1.3.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251111163417-95abcf5c77ba // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1 // indirect
)

replace github.com/relab/gorums => ../

tool (
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
