module github.com/relab/gorums/examples

go 1.14

require (
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/relab/gorums v0.3.0
	golang.org/x/term v0.0.0-20210317153231-de623e64d2a6
	google.golang.org/grpc v1.36.1
	google.golang.org/protobuf v1.26.0
)

replace github.com/relab/gorums => ../
