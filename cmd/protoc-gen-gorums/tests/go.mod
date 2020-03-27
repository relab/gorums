module github.com/relab/gorums/cmd/protoc-gen-gorums/tests

go 1.13

require (
	github.com/golang/protobuf v1.4.0-rc.1
	github.com/relab/gorums v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a
	google.golang.org/grpc v1.28.0
	google.golang.org/protobuf v0.0.0-20200214004509-aa735f3ccadf
)

replace github.com/relab/gorums => ../../..
