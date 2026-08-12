module github.com/relab/gorums/benchkit

go 1.26.2

require (
	github.com/google/pprof v0.0.0-20260604005048-7023385849c0
	github.com/relab/gorums v0.11.0
	github.com/relab/iago v0.0.0-20260702190239-acea5b94dd97
	golang.org/x/exp v0.0.0-20260611194520-c48552f49976
	golang.org/x/sync v0.21.0
	google.golang.org/grpc v1.82.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/pkg/sftp v1.13.10 // indirect
	github.com/relab/wrfs v0.0.0-20220416082020-a641cd350078 // indirect
	go.uber.org/goleak v1.3.0 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260630182238-925bb5da69e7 // indirect
)

// benchkit tracks the gorums source in this repository rather than a released
// version, so that a change to the gorums API and its benchkit follow-up land
// together. The target is inside this repository, so every clone resolves it.
// Extracting benchkit to its own repository replaces this with a version pin.
replace github.com/relab/gorums => ../
