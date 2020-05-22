module github.com/relab/gorums

go 1.14

require (
	github.com/golang/protobuf v1.4.2
	github.com/google/go-cmp v0.4.0
	golang.org/x/net v0.0.0-20200301022130-244492dfa37a
	golang.org/x/tools v0.0.0-20200304193943-95d2e580d8eb
	google.golang.org/grpc v1.29.1
	google.golang.org/protobuf v1.23.0
)

replace google.golang.org/grpc => github.com/meling/grpc-go v1.30.2
