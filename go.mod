module github.com/relab/gorums

go 1.27.2

require (
	github.com/google/go-cmp v0.7.0
	go.uber.org/goleak v1.3.0
	golang.org/x/tools v0.51.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/stretchr/testify v1.12.1 // indirect
	golang.org/x/exp v0.0.0-20261007192929-f45ad48fbe92 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2 // indirect
)

tool (
	golang.org/x/exp/cmd/gorelease
	golang.org/x/tools/cmd/stress
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
