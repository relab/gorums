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
strictordering			:= internal/strictordering

.PHONY: dev download install-tools installgorums clean

dev: installgorums
	@echo Generating Gorums code for zorums.proto as a multiple _gorums.pb.go files in dev folder
	rm -f $(dev_path)/*.pb.go
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

gorums_grpc.pb.go: gorums.proto
	@echo Generating gorums GRPC services
	@protoc --go-grpc_out=paths=source_relative:. gorums.proto

$(strictordering)/strictordering.pb.go: $(strictordering)/strictordering.proto
	@echo Generating strictordering proto options
	@protoc --go_out=paths=source_relative:. $(strictordering)/strictordering.proto

installgorums: gorums.pb.go gorums_grpc.pb.go $(strictordering)/strictordering.pb.go
	@echo Installing protoc-gen-gorums compiler plugin for protoc
	@go install github.com/relab/gorums/cmd/protoc-gen-gorums

bundle: installgorums $(static_file)

$(static_file): $(static_files)
	cp $(static_file) $(static_file).bak
	protoc -I$(dev_path):. --gorums_out=bundle=$(static_file):. $(zorums_proto)

clean:
	rm -f $(static_file).bak

.PHONY: gentests $(test_files)

gentests: $(test_files) $(failing_test_files)

$(test_files): installgorums
	@protoc -I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative,trace=true:. $@ \
	|| (echo "unexpected failure with exit code: $$?")

$(failing_test_files): installgorums
	@protoc -I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative,trace=true:. $@ \
	&& (echo "expected protoc to fail but got exit code: $$?")
