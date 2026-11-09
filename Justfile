# Development tasks for Gorums. Run `just` to list the recipes.
#
# All recipes run from the repository root, which holds the workspace
# (go.work) of the root, benchkit, and examples modules.

set quiet
set shell := ["bash", "-euo", "pipefail", "-c"]

# --- Paths and packages ------------------------------------------------------

plugin_dir := "./cmd/protoc-gen-gorums"
dev_dir := plugin_dir / "dev"
static_file := plugin_dir / "gengorums/template_static.go"
packages := "./... ./examples/... ./benchkit/..."

# The benchkit module keeps its .proto files under benchkit/proto so that the
# import paths protoc records stay "benchkit/*.proto" and "benchmark/*.proto".
bk_module := "github.com/relab/gorums/benchkit"
bk_proto_path := "benchkit/proto:third_party:."

# --- Code generators ---------------------------------------------------------
#
# The generators are the tools pinned in go.mod, including protoc-gen-gorums
# from this tree. `go tool -n` prints the path of the cached binary and
# rebuilds it first when its sources changed, so a recipe never runs a stale
# plugin. The $(...) is expanded by the shell, once per protoc invocation.

go_plugin := '--plugin=protoc-gen-go="$(go tool -n protoc-gen-go)"'
grpc_plugin := '--plugin=protoc-gen-go-grpc="$(go tool -n protoc-gen-go-grpc)"'
gorums_plugin := '--plugin=protoc-gen-gorums="$(go tool -n protoc-gen-gorums)"'

# --- Generate ----------------------------------------------------------------

# List the recipes.
[private]
default:
    just --list

# Regenerate all generated code and build the benchmark, sweep, and storage binaries.
all: genproto benchmark sweep storage

# Regenerate every generated file in the repository.
[group('generate')]
genproto: static _generate

# The generators are independent of each other once the static code is bundled.
[parallel]
[private]
_generate: dev runtime benchkit-proto tests-proto examples-proto

# Fail if regenerating changes a generated file; uncommitted edits elsewhere are fine.
[group('generate')]
check-generated: genproto
    git diff --exit-code --stat -- '*.pb.go' '*.golden' {{ static_file }}

# Bundle the static files of the dev directory into the generator's template_static.go.
[group('generate')]
static:
    cp {{ static_file }} {{ static_file }}.bak
    go tool protoc-gen-gorums --bundle={{ static_file }}

# Regenerate the zorums_*.pb.go files of the generator's own tests.
[group('generate')]
dev: static
    rm -f {{ dev_dir }}/zorums*.pb.go
    protoc -I={{ dev_dir }}:third_party:. {{ go_plugin }} {{ gorums_plugin }} \
        --go_out=:. --gorums_out=dev=true:. \
        --go_opt=default_api_level=API_OPAQUE \
        {{ dev_dir }}/zorums.proto

# Regenerate the stream protocol code that the runtime uses.
[group('generate')]
runtime:
    protoc -I={{ dev_dir }}:third_party:. {{ go_plugin }} {{ grpc_plugin }} \
        --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. \
        --go_opt=default_api_level=API_OPAQUE \
        internal/stream/stream.proto

# Regenerate the benchkit and benchmark protocol code.
[group('generate')]
benchkit-proto:
    # It is written into the module root rather than next to the .proto files.
    protoc -I={{ bk_proto_path }} {{ go_plugin }} \
        --go_out=benchkit --go_opt=module={{ bk_module }} \
        --go_opt=default_api_level=API_OPAQUE \
        benchkit/proto/benchkit/benchkit.proto
    for p in benchkit/control benchmark/benchmark; do \
        protoc -I={{ bk_proto_path }} {{ go_plugin }} {{ gorums_plugin }} \
            --go_out=benchkit --go_opt=module={{ bk_module }} \
            --go_opt=default_api_level=API_OPAQUE \
            --gorums_out=benchkit --gorums_opt=module={{ bk_module }} \
            "benchkit/proto/${p%/*}/${p#*/}.proto"; \
    done

# Regenerate the protocol code of the integration tests in internal/tests.
[group('generate')]
[working-directory('internal/tests')]
tests-proto:
    # protoc-gen-gorums also writes gorums.pb.go, which declares the Config, ID,
    # and Node aliases, next to the first _gorums.pb.go file of each Go package.
    for p in */*.proto; do \
        protoc -I=../..:. {{ go_plugin }} {{ gorums_plugin }} \
            --go_out=paths=source_relative:. --gorums_out=paths=source_relative:. \
            --go_opt=default_api_level=API_OPAQUE "$p"; \
    done

# Regenerate the protocol code of the examples.
[group('generate')]
[working-directory('examples')]
examples-proto:
    protoc -I="{{ justfile_directory() }}:." {{ go_plugin }} {{ gorums_plugin }} \
        --go_out=paths=source_relative:. --gorums_out=paths=source_relative:. \
        --go_opt=default_api_level=API_OPAQUE \
        storage/proto/storage.proto

# Rewrite the generator golden files in internal/testprotos/testdata.
[group('generate')]
golden:
    # Run it after an intended change to a template, and review the diff.
    go test ./internal/testprotos -run TestGoldenProtoFiles -count=1 -update

# --- Build -------------------------------------------------------------------

# Build the benchmark binary.
[group('build')]
benchmark:
    go build -C benchkit -o cmd/benchmark/benchmark ./cmd/benchmark

# Build the sweep binary.
[group('build')]
sweep:
    go build -C benchkit/cmd/sweep -o sweep .

# Build the storage example. Extra arguments go to `go build`.
[group('examples')]
storage *flags:
    go build -C examples -o storage/storage {{ flags }} ./storage

# Build the storage example without optimizations or inlining, for debuggers.
[group('examples')]
storage-debug: (storage "-gcflags='all=-N -l'")

# Start a three-node local storage cluster; Ctrl-C stops all servers.
[group('examples')]
storage-start: storage
    examples/storage/storage -cluster -addrs localhost:9081,localhost:9082,localhost:9083

# Run the sweep smoke test on a throwaway Docker cluster of n peer containers.
[group('build')]
smoke-docker n="5":
    # It needs Docker and rsync >= 3.2.4. The verdict is written to
    # benchkit/out/smoke-docker/verdict.md.
    SMOKE_N={{ n }} ./benchkit/smoke/docker/run.sh

# --- Test --------------------------------------------------------------------

# Run the unit tests, then check that the benchmarks still run.
[group('test')]
test *flags: && benchtest
    go test {{ flags }} {{ packages }}

# Run the tests against real TCP connections instead of in-memory ones.
[group('test')]
integrationtest *flags:
    go test -tags=integration {{ flags }} {{ packages }}

# Run the tests with the race detector on one, two, and four CPUs.
[group('test')]
testrace *flags:
    go test -race -cpu=1,2,4 {{ flags }} {{ packages }}

# Run the ordering tests for a fixed duration instead of a fixed iteration count.
[group('test')]
stresstest:
    # The -stress flag exists only in internal/tests/ordering.
    go test -count=1 ./internal/tests/ordering -stress

# Check that every benchmark runs; show its output only on failure.
[group('test')]
benchtest:
    #!/usr/bin/env bash
    set -euo pipefail
    # A fixed 100 iterations avoids port exhaustion on macOS.
    out=$(mktemp)
    trap 'rm -f "$out"' EXIT
    if ! go test -run='^$' -bench=. -benchtime=100x -count=1 {{ packages }} > "$out" 2>&1; then
        echo "Benchmark validation failed:"
        cat "$out"
        exit 1
    fi
    echo "Benchmark validation passed"

# Measure the benchmarks with longer runs. Raise -count for statistical significance.
[group('test')]
bench:
    # It covers the main module and internal/tests/oneway; add new benchmark packages here.
    go test -run='^$' -bench=. -benchtime=1s -count=10 . ./internal/tests/oneway

# --- Lint --------------------------------------------------------------------

# Run buf, golangci-lint, and deadcode.
[group('lint')]
lint: deadcode
    # The golangci-lint version must match the one that .github/workflows/golangci-lint.yml pins.
    {{ require("buf") }} lint
    {{ require("golangci-lint") }} run {{ packages }}

# Report functions that no main or test in any workspace module reaches.
[group('lint')]
deadcode:
    # The report is advisory: exported library API without an in-repo caller and
    # example-only helpers are expected. Review new entries for dead internal code.
    go tool deadcode -test {{ packages }}

# Apply the Go modernizers.
[group('lint')]
modernize:
    go fix {{ packages }}
    go tool modernize -fix {{ packages }}

# Report every gopls diagnostic, including hints, in non-generated Go files.
[group('lint')]
goplscheck:
    #!/usr/bin/env bash
    set -euo pipefail
    gopls={{ require("gopls") }}
    out=$(mktemp)
    trap 'rm -f "$out"' EXIT
    if ! git ls-files -z --cached --others --exclude-standard -- '*.go' \
        ':(exclude)*.pb.go' ':(exclude)**/*.pb.go' \
        ':(exclude)cmd/protoc-gen-gorums/gengorums/template_static.go' \
        | xargs -0 "$gopls" check -severity=hint > "$out"; then
        cat "$out"
        exit 1
    fi
    if [ -s "$out" ]; then
        cat "$out"
        echo
        echo "gopls diagnostics found; apply the suggested quick fixes and rerun just goplscheck."
        exit 1
    fi
