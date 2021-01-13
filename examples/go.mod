module github.com/relab/gorums/examples

go 1.14

require (
	github.com/golang/protobuf v1.4.3
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/relab/gorums v0.2.1
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
	google.golang.org/grpc v1.34.0
	google.golang.org/protobuf v1.25.0
)

replace github.com/relab/gorums => ../
