proto_include := $(shell go list -m -f {{.Dir}} github.com/relab/gorums)
proto_src := $(shell find . -type f -name '*.proto')
proto_go := $(proto_src:%.proto=%.pb.go)
gorums_go := $(proto_src:%.proto=%_gorums.pb.go)
binaries := storage/storage

.PHONY: all debug proto download tools $(binaries)

all: $(binaries)

# Build binaries in debug mode: Tell the Go compiler to compile without
# optimizations (-N) or inlining (-l) so that debuggers, stack traces,
# and variable inspection behave exactly like the source code.
debug: GCFLAGS += -gcflags='all=-N -l'
debug: $(binaries)

# Build binaries.
$(binaries): proto
	@go build -o ./$@ $(GCFLAGS) ./$(dir $@)

# Generate Go files from proto files.
proto: tools download $(proto_go) $(gorums_go)

download:
	@go mod download

tools: download
	@go install tool

%.pb.go %_gorums.pb.go : %.proto
	@protoc -I="$(proto_include):." \
		--go_out=paths=source_relative:. \
		--gorums_out=paths=source_relative:. \
		--go_opt=default_api_level=API_OPAQUE \
		$<
