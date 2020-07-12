PLUGIN_PATH				:= ./cmd/protoc-gen-gorums
dev_path				:= $(PLUGIN_PATH)/dev
gen_path				:= $(PLUGIN_PATH)/gengorums
gen_files				:= $(shell find $(gen_path) -name "*.go" -not -name "*_test.go")
zorums_proto			:= $(dev_path)/zorums.proto
static_file				:= $(gen_path)/template_static.go
static_files			:= $(shell find $(dev_path) -name "*.go" -not -name "zorums*" -not -name "*_test.go")
proto_path 				:= $(dev_path):third_party:.

plugin_deps				:= gorums.pb.go internal/ordering/opts.pb.go internal/correctable/opts.pb.go $(static_file)
benchmark_deps			:= benchmark/benchmark.pb.go benchmark/benchmark_grpc.pb.go benchmark/benchmark_gorums.pb.go

.PHONY: dev tools bootstrapgorums installgorums benchmark test

dev: installgorums ordering/ordering.pb.go ordering/ordering_grpc.pb.go
	@rm -f $(dev_path)/zorums*.pb.go
	@protoc -I=$(proto_path) \
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
	@protoc -I=$(proto_path) --go_out=paths=source_relative:. $^

%_grpc.pb.go : %.proto
	@protoc -I=$(proto_path) --go-grpc_out=paths=source_relative:. $^

%_gorums.pb.go : %.proto
	@protoc -I=$(proto_path) --gorums_out=paths=source_relative,trace=true:. $^

tools:
	@go mod download
	@cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -I % go install %

installgorums: bootstrapgorums $(gen_files) $(plugin_deps) Makefile
	@go install $(PLUGIN_PATH)

ifeq (, $(shell which protoc-gen-gorums))
bootstrapgorums: tools
	@echo "Bootstrapping gorums plugin"
	@go install github.com/relab/gorums/cmd/protoc-gen-gorums
endif

test: installgorums
	@go test -v $(dev_path)
	@$(MAKE) --no-print-directory -C ./tests -B runtests
