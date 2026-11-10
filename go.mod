module github.com/relab/gorums

go 1.27.3

require (
	github.com/google/go-cmp v0.7.0
	go.uber.org/goleak v1.3.0
	golang.org/x/tools v0.52.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261110000943-09e21b237f2c
	google.golang.org/grpc v1.85.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/stretchr/testify v1.12.1 // indirect
	golang.org/x/exp v0.0.0-20261014143641-f0251eb4aa3e // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/telemetry v0.0.0-20261009151841-97e8ea77aa2a // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2 // indirect
)

tool (
	github.com/relab/gorums/cmd/protoc-gen-gorums
	golang.org/x/exp/cmd/gorelease
	golang.org/x/tools/cmd/deadcode
	golang.org/x/tools/cmd/stress
	golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
