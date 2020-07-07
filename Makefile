PLUGIN_PATH				:= ./cmd/protoc-gen-gorums
dev_path				:= $(PLUGIN_PATH)/dev
gen_path				:= $(PLUGIN_PATH)/gengorums
tests_path				:= internal/testprotos
zorums_proto			:= $(dev_path)/zorums.proto
tests_zorums_proto		:= $(tests_path)/calltypes/zorums/zorums.proto
tests_zorums_gen		:= $(patsubst %.proto,%_gorums.pb.go,$(tests_zorums_proto))
gen_files				:= $(shell find $(dev_path) -name "zorums*.pb.go")
static_file				:= $(gen_path)/template_static.go
static_files			:= $(shell find $(dev_path) -name "*.go" -not -name "zorums*" -not -name "*_test.go")
test_files				:= $(shell find $(tests_path) -name "*.proto" -not -path "*failing*")
failing_test_files		:= $(shell find $(tests_path) -name "*.proto" -path "*failing*")
test_gen_files			:= $(patsubst %.proto,%_gorums.pb.go,$(test_files))
tmp_dir					:= $(shell mktemp -d -t gorums-XXXXX)

plugin_deps				:= gorums.pb.go internal/ordering/opts.pb.go internal/correctable/opts.pb.go $(static_file)
benchmark_deps			:= benchmark/benchmark.pb.go benchmark/benchmark_grpc.pb.go benchmark/benchmark_gorums.pb.go

.PHONY: dev tools bootstrapgorums installgorums benchmark

dev: installgorums ordering/ordering.pb.go ordering/ordering_grpc.pb.go
	@rm -f $(dev_path)/zorums*.pb.go
	@protoc -I$(dev_path):. \
		--go_out=:. \
		--go-grpc_out=:. \
		--gorums_out=dev=true,trace=true:. \
		$(zorums_proto)

benchmark: installgorums $(benchmark_deps)
	@go build -o cmd/benchmark/benchmark ./cmd/benchmark

$(static_file): $(static_files)
	@cp $(static_file) $(static_file).bak
	@protoc-gen-gorums --bundle=$(static_file)

%.pb.go : %.proto
	@protoc --go_out=paths=source_relative:. $^

%_grpc.pb.go : %.proto
	@protoc --go-grpc_out=paths=source_relative:. $^

%_gorums.pb.go : %.proto
	@protoc --gorums_out=paths=source_relative,trace=true:. $^

tools:
	@go mod download
	@cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -I % go install %

installgorums: bootstrapgorums $(plugin_deps) Makefile
	@go install $(PLUGIN_PATH)

ifeq (, $(shell which protoc-gen-gorums))
bootstrapgorums: tools
	@echo "Bootstrapping gorums plugin"
	@go install github.com/relab/gorums/cmd/protoc-gen-gorums
endif

.PHONY: gentests $(test_files)
gentests: $(test_files) $(failing_test_files) stability

$(test_files): installgorums
	@echo "Running protoc test with source files expected to pass"
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

.PHONY: stability
stability: installgorums
	@echo "Running protoc test with source files expected to remain stable (no output change between runs)"
	@protoc -I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative,trace=true:. $(tests_zorums_proto) \
	|| (echo "unexpected failure with exit code: $$?")
	@cp $(tests_zorums_gen) $(tmp_dir)/x_gorums.pb.go
	@protoc -I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative,trace=true:. $(tests_zorums_proto) \
	|| (echo "unexpected failure with exit code: $$?")
	@diff $(tests_zorums_gen) $(tmp_dir)/x_gorums.pb.go \
	|| (echo "unexpected instability, observed changes between protoc runs: $$?")
	@rm -rf $(tmp_dir)
