PLUGIN_PATH				:= cmd/protoc-gen-gorums
dev_path				:= $(PLUGIN_PATH)/dev
gen_path				:= $(PLUGIN_PATH)/gengorums
tests_path				:= $(PLUGIN_PATH)/tests
zorums_proto			:= $(dev_path)/zorums.proto
gen_files				:= $(shell find $(dev_path) -name "zorums*.pb.go")
static_file				:= $(gen_path)/template_static.go
static_files			:= $(shell find $(dev_path) -name "*.go" -not -name "zorums*" -not -name "*_test.go")
test_files				:= $(shell find $(tests_path) -name "*.proto" -not -path "*failing*")
failing_test_files		:= $(shell find $(tests_path) -name "*.proto" -path "*failing*")
test_gen_files			:= $(patsubst %.proto,%_gorums.pb.go,$(test_files))
internal_so				:= internal/ordering
public_so				:= ordering/

.PHONY: dev download install-tools installgorums clean

dev: installgorums $(public_so)/ordering.pb.go $(public_so)/ordering_grpc.pb.go
	@echo Generating Gorums code for zorums.proto as a multiple _gorums.pb.go files in dev folder
	rm -f $(dev_path)/zorums*.pb.go
	protoc -I$(dev_path):. \
		--go_out=:. \
		--go-grpc_out=:. \
		--gorums_out=dev=true,trace=true:. \
		$(zorums_proto)

# TODO(meling) remove this when v2 released; for now, it is necessary for compiling gorums.proto
.PHONY: getv1
getv1:
	@echo getting v1 pre-release proto package based on v2 impl
	@go mod tidy
	@go get -u github.com/golang/protobuf/proto@api-v1

download:
	@echo Download go.mod dependencies
	@go mod download

install-tools: download
	@echo Installing tools from tools.go
	@cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

gorums.pb.go: gorums.proto
	@echo Generating gorums proto options
	@protoc --go_out=paths=source_relative:. gorums.proto

$(internal_so)/opts.pb.go: $(internal_so)/opts.proto
	@echo Generating ordering proto options
	@protoc --go_out=paths=source_relative:. $(internal_so)/opts.proto

$(public_so)/ordering.pb.go: $(public_so)/ordering.proto
	@echo Generating ordering protocol buffers
	@protoc --go_out=paths=source_relative:. $(public_so)/ordering.proto

$(public_so)/ordering_grpc.pb.go: $(public_so)/ordering.proto
	@echo Generating ordering gRPC service
	@protoc --go-grpc_out=paths=source_relative:. $(public_so)/ordering.proto

installgorums: gorums.pb.go $(internal_so)/opts.pb.go
	@echo Installing protoc-gen-gorums compiler plugin for protoc
	@go install github.com/relab/gorums/cmd/protoc-gen-gorums

bundle: installgorums $(static_file)

$(static_file): $(static_files)
	cp $(static_file) $(static_file).bak
	protoc -I$(dev_path):. --gorums_out=bundle=$(static_file):. $(zorums_proto)

bootbundle:
	protoc -I$(dev_path):. --gorums_out=bundle=$(static_file):. gorums.proto

clean:
	rm -f $(static_file).bak

.PHONY: gentests $(test_files)

gentests: $(test_files) $(failing_test_files)

$(test_files): installgorums
	@echo Running protoc test with source files expected to pass
	@protoc -I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative,trace=true:. $@ \
	|| (echo "unexpected failure with exit code: $$?")

$(failing_test_files): installgorums
	@echo "Running protoc test with source files expected to fail (output is suppressed)"
	@protoc -I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative,trace=true:. $@ 2> /dev/null \
	&& (echo "expected protoc to fail but got exit code: $$?") || (exit 0)
