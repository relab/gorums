module github.com/relab/gorums/examples

go 1.14

require (
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/relab/gorums v0.4.0
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56
	google.golang.org/grpc v1.37.0
	google.golang.org/protobuf v1.26.0
)

replace github.com/relab/gorums => ../
