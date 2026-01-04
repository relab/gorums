module github.com/relab/gorums

go 1.25.5

require (
	github.com/google/go-cmp v0.7.0
	go.uber.org/goleak v1.3.0
	golang.org/x/sync v0.19.0
	golang.org/x/tools v0.40.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251222181119-0a764e51fe1b
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/exp v0.0.0-20251219203646-944ab1f22d93 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.0 // indirect
)

tool (
	golang.org/x/exp/cmd/gorelease
	golang.org/x/tools/cmd/stress
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
