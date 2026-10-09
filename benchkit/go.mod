module github.com/relab/gorums/benchkit

go 1.27.2

require (
	github.com/google/pprof v0.0.0-20261008003335-7bae8d8c4c9e
	github.com/relab/gorums v0.11.0
	github.com/relab/iago v0.0.0-20260715045113-2b8915a614a8
	golang.org/x/exp v0.0.0-20261007192929-f45ad48fbe92
	golang.org/x/sync v0.24.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/pkg/sftp v1.13.11 // indirect
	github.com/relab/wrfs v0.0.0-20220416082020-a641cd350078 // indirect
	go.uber.org/goleak v1.3.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
)

// benchkit tracks the gorums source in this repository rather than a released
// version, so that a change to the gorums API and its benchkit follow-up land
// together. The target is inside this repository, so every clone resolves it.
// Extracting benchkit to its own repository replaces this with a version pin.
replace github.com/relab/gorums => ../
