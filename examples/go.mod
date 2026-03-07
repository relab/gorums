module github.com/relab/gorums/examples

go 1.26.0

require (
	github.com/relab/gorums v0.11.0
	golang.org/x/term v0.40.0
	google.golang.org/grpc v1.79.2
	google.golang.org/protobuf v1.36.11
)

require (
	go.uber.org/goleak v1.3.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.1 // indirect
)

replace github.com/relab/gorums => ../

tool (
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
