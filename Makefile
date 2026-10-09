PLUGIN_PATH				:= ./cmd/protoc-gen-gorums
dev_path				:= $(PLUGIN_PATH)/dev
gen_path				:= $(PLUGIN_PATH)/gengorums
gen_files				:= $(shell find $(gen_path) -name "*.go" -not -name "*_test.go")
zorums_proto			:= $(dev_path)/zorums.proto
static_file				:= $(gen_path)/template_static.go
static_files			:= $(shell find $(dev_path) -name "*.go" -not -name "zorums*" -not -name "*_test.go")
proto_path 				:= $(dev_path):third_party:.

# The benchkit module keeps its .proto files under benchkit/proto so that the
# import paths protoc records stay "benchkit/*.proto" and "benchmark/*.proto".
bk_path					:= benchkit/proto
bk_proto_path			:= $(bk_path):third_party:.
bk_module				:= github.com/relab/gorums/benchkit
workspace_packages		:= ./... ./examples/... ./benchkit/...

plugin_deps				:= gorums.pb.go $(static_file)
runtime_deps			:= internal/stream/stream.pb.go internal/stream/stream_grpc.pb.go
benchkit_deps			:= benchkit/benchkit.pb.go benchkit/control.pb.go benchkit/control_gorums.pb.go
benchmark_deps			:= $(benchkit_deps) benchkit/benchmark/benchmark.pb.go benchkit/benchmark/benchmark_gorums.pb.go

.PHONY: all dev tools bootstrapgorums installgorums benchmark sweep test compiletests genproto benchtest bench lint deadcode modernize goplscheck

all: dev benchmark compiletests

dev: installgorums $(runtime_deps)
	@rm -f $(dev_path)/zorums*.pb.go
	@protoc -I=$(proto_path) \
		--go_out=:. \
		--gorums_out=dev=true:. \
		--go_opt=default_api_level=API_OPAQUE \
		$(zorums_proto)

benchmark: installgorums $(benchmark_deps)
	@go build -C benchkit -o cmd/benchmark/benchmark ./cmd/benchmark

benchkit: installgorums $(benchkit_deps)

sweep: $(benchkit_deps)
	@go build -C benchkit/cmd/sweep -o sweep .

# The benchkit module's generated code is written back into the module root
# rather than next to its .proto file, so these cannot use the pattern rules.
benchkit/benchkit.pb.go: $(bk_path)/benchkit/benchkit.proto
	@protoc -I=$(bk_proto_path) \
		--go_out=benchkit --go_opt=module=$(bk_module) \
		--go_opt=default_api_level=API_OPAQUE $<

benchkit/control.pb.go: $(bk_path)/benchkit/control.proto
	@protoc -I=$(bk_proto_path) \
		--go_out=benchkit --go_opt=module=$(bk_module) \
		--go_opt=default_api_level=API_OPAQUE $<

benchkit/control_gorums.pb.go: $(bk_path)/benchkit/control.proto
	@protoc -I=$(bk_proto_path) \
		--gorums_out=benchkit --gorums_opt=module=$(bk_module) $<

benchkit/benchmark/benchmark.pb.go: $(bk_path)/benchmark/benchmark.proto
	@protoc -I=$(bk_proto_path) \
		--go_out=benchkit --go_opt=module=$(bk_module) \
		--go_opt=default_api_level=API_OPAQUE $<

benchkit/benchmark/benchmark_gorums.pb.go: $(bk_path)/benchmark/benchmark.proto
	@protoc -I=$(bk_proto_path) \
		--gorums_out=benchkit --gorums_opt=module=$(bk_module) $<

$(static_file): $(static_files)
	@cp $(static_file) $(static_file).bak
	@protoc-gen-gorums --bundle=$(static_file)

%.pb.go : %.proto
	@protoc -I=$(proto_path) \
		--go_opt=default_api_level=API_OPAQUE \
		--go_out=paths=source_relative:. $^

%_grpc.pb.go : %.proto
	@protoc -I=$(proto_path) --go-grpc_out=paths=source_relative:. $^

%_gorums.pb.go : %.proto
	@protoc -I=$(proto_path) --gorums_out=paths=source_relative:. $^

tools:
	@go install tool

installgorums: bootstrapgorums $(gen_files) $(plugin_deps) Makefile
	@go install $(PLUGIN_PATH)

ifeq (, $(shell which protoc-gen-gorums))
bootstrapgorums: tools
	@echo "Bootstrapping gorums plugin"
	@go install github.com/relab/gorums/cmd/protoc-gen-gorums
endif

compiletests: installgorums
	@$(MAKE) --no-print-directory -C ./internal/tests all

test: compiletests benchtest
	@go test $(workspace_packages)

integrationtest: compiletests
	@go test -tags=integration $(workspace_packages)

testrace: compiletests
	go test -race -cpu=1,2,4 $(workspace_packages)

# Run benchmarks with validation (short runs to verify they don't fail).
# Uses -benchtime=100x for limited iterations to avoid port exhaustion on macOS.
# Suppresses output on success; shows details on failure.
benchtest: compiletests
	@if ! go test -run=^$$ -bench=. -benchtime=100x -count=1 $(workspace_packages) > /tmp/benchtest.out 2>&1; then \
		echo "Benchmark validation failed:"; \
		cat /tmp/benchtest.out; \
		exit 1; \
	fi
	@echo "Benchmark validation passed"

# Run benchmarks with proper measurement (longer runs for performance analysis).
# Use -count=10 or more for statistically significant results.
# This only runs benchmarks in the main module and in internal/tests/oneway
# (when adding benchmarks elsewhere, update this target accordingly).
bench: compiletests
	go test -run=^$$ -bench=. -benchtime=1s -count=10 . ./internal/tests/oneway

# Run the ordering tests for a fixed duration instead of a fixed number of
# iterations. The -stress test flag is defined only in internal/tests/ordering.
stresstest: compiletests
	go test -count=1 ./internal/tests/ordering -stress

# Warning: will probably run for 10 minutes; the timeout does not work
stressdev: tools
	go test -c $(dev_path)
	stress -timeout=5s -p=1 ./dev.test

# Warning: should not be aborted (CTRL-C), as otherwise it may
# leave behind compiled files in-place.
# Again the timeout does not work, so it will probably leave behind generated files.
stressgen: tools
	cd ./internal/testprotos; go test -c
	cd ./internal/testprotos; stress -timeout=10s -p=1 ./testprotos.test
	rm ./internal/testprotos/testprotos.test

# Use the golangci-lint version that .github/workflows/golangci-lint.yml pins,
# so make lint and CI agree.
lint: deadcode
	@golangci-lint run ./... ./examples/... ./benchkit/...

# deadcode reports functions unreachable from any main or test across all
# workspace modules (root, examples, benchkit), so cross-module usage is
# accounted for. It is advisory: exported library API with no in-repo caller
# (e.g. optional dial/server options) and example-only helpers are expected to
# appear. Review new entries for genuinely dead internal code.
deadcode:
	@go run golang.org/x/tools/cmd/deadcode@latest -test ./... ./examples/... ./benchkit/...

modernize:
	@go fix ./... ./examples/... ./benchkit/...
	@go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest \
		-fix ./... ./examples/... ./benchkit/...

# Report all gopls diagnostics, including hint-level style and modernization
# suggestions. Generated Go files are excluded because their generators own them.
goplscheck:
	@command -v gopls >/dev/null || { echo "gopls is required; install it from https://go.dev/gopls/"; exit 1; }
	@out=$$(mktemp); trap 'rm -f "$$out"' EXIT; \
		if ! git ls-files -z --cached --others --exclude-standard -- '*.go' \
			':(exclude)*.pb.go' ':(exclude)**/*.pb.go' \
			':(exclude)cmd/protoc-gen-gorums/gengorums/template_static.go' \
			| xargs -0 gopls check -severity=hint > "$$out"; then \
			cat "$$out"; \
			exit 1; \
		fi; \
		if [ -s "$$out" ]; then \
			cat "$$out"; \
			echo ""; \
			echo "gopls diagnostics found; apply the suggested quick fixes and rerun make goplscheck."; \
			exit 1; \
		fi

# Regenerate all Gorums and protobuf generated files across the repo (dev, benchmark, internal/tests, examples).
# This will force regeneration even though the proto files have not changed.
genproto: installgorums dev
	@echo "Regenerating all proto files (dev, benchkit, benchmark, internal/tests, examples)"
	@$(MAKE) -B -s dev
	@$(MAKE) -B -s $(benchmark_deps)
	@$(MAKE) -B -s --no-print-directory -C ./internal/tests all
	@$(MAKE) -B -s --no-print-directory -C ./examples all
