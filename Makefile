PLUGIN_PATH				:= ./cmd/protoc-gen-gorums
dev_path				:= $(PLUGIN_PATH)/dev
gen_path				:= $(PLUGIN_PATH)/gengorums
gen_files				:= $(shell find $(gen_path) -name "*.go" -not -name "*_test.go")
zorums_proto			:= $(dev_path)/zorums.proto
static_file				:= $(gen_path)/template_static.go
static_files			:= $(shell find $(dev_path) -name "*.go" -not -name "zorums*" -not -name "*_test.go")
proto_path 				:= $(dev_path):third_party:.

plugin_deps				:= gorums.pb.go internal/correctable/opts.pb.go $(static_file)
runtime_deps			:= ordering/ordering.pb.go ordering/ordering_grpc.pb.go
benchmark_deps			:= benchmark/benchmark.pb.go benchmark/benchmark_gorums.pb.go

.PHONY: all dev tools bootstrapgorums installgorums benchmark test compiletests genproto

all: dev benchmark compiletests

dev: installgorums $(runtime_deps)
	@rm -f $(dev_path)/zorums*.pb.go
	@protoc -I=$(proto_path) \
		--go_out=:. \
		--gorums_out=dev=true:. \
		--go_opt=default_api_level=API_OPAQUE \
		$(zorums_proto)

benchmark: installgorums $(benchmark_deps)
	@go build -o cmd/benchmark/benchmark ./cmd/benchmark

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
	@$(MAKE) --no-print-directory -C ./tests all

test: compiletests
	@go test ./...

testrace: compiletests
	go test -race -cpu=1,2,4 ./...

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

# Regenerate all Gorums and protobuf generated files across the repo (dev, benchmark, tests, examples).
# This will force regeneration even though the proto files have not changed.
genproto:
	@echo "Regenerating all proto files (dev, benchmark, tests, examples)"
	@$(MAKE) -B -s dev
	@$(MAKE) -B -s benchmark
	@$(MAKE) -B -s --no-print-directory -C ./tests all
	@$(MAKE) -B -s --no-print-directory -C ./examples all

# Release helper targets to automate common release preparation steps.
# See doc/release-guide.md for details.
.PHONY: release-tools prepare-release release-pr release-publish

release-tools:
	@echo "+ Checking for required release tools (gorelease, gh)..."
	@command -v gorelease >/dev/null 2>&1 || go install golang.org/x/exp/cmd/gorelease@latest
	@command -v gh >/dev/null || (echo "Please install 'gh' (GitHub CLI): brew install gh" && exit 1)
	@echo "+ Checking installed tool versions"
	@protoc --version || echo "protoc not found or not on PATH"
	@protoc-gen-go --version || echo "protoc-gen-go not found or not on PATH"
	@protoc-gen-go-grpc --version || echo "protoc-gen-go-grpc not found or not on PATH"
	@protoc-gen-gorums --version || echo "protoc-gen-gorums not found or not on PATH"
	@echo "+ OK"

prepare-release: release-tools
	@echo "+ Upgrade module dependencies"
	@go get -u ./... && go mod tidy
	@cd examples && go get -u ./... && go mod tidy
	@$(MAKE) genproto
	@echo "+ Running gorelease to suggest the next version"
	@tmp=$$(mktemp); \
	gorelease | tee $$tmp; \
	suggested=$$(awk -F': ' '/^Suggested version:/ {print $$2; exit}' $$tmp); \
	echo "--------------------------------------------------------------------------"; \
	echo ""; \
	echo "gorelease suggests: $$suggested"; \
	echo ""; \
	echo "If the suggested version looks good, please edit the version constants in:"; \
	echo "  - internal/version/version.go"; \
	echo "  - version.go"; \
	echo ""; \
	echo "After editing those files, re-run to update generated files before creating the PR:"; \
	echo "  make genproto"; \
	echo "  go get -C examples github.com/relab/gorums@$${suggested:-vX.Y.Z}"; \
	echo "  go mod tidy && (cd examples && go mod tidy)"; \
	echo ""; \
	echo "Then create the release PR:"; \
	echo "  make release-pr VERSION=$${suggested:-vX.Y.Z}"

release-pr:
	@if [ -z "$(VERSION)" ]; then echo "Usage: make release-pr VERSION=v0.9.0"; exit 1; fi
	@if [ -n "$$(git status --porcelain)" ]; then echo "Uncommitted changes present; commit or stash first; aborting."; exit 1; fi
	@BRANCH="release/$(VERSION)-devel"; \
	git switch -c $$BRANCH; \
	git add -A; \
	git commit -m "Gorums release $(VERSION)"; \
	git push -u origin HEAD; \
	gh pr create --title "Gorums release $(VERSION)" --body "Release $(VERSION)"

release-publish:
	@if [ -z "$(VERSION)" ]; then echo "Usage: make release-publish VERSION=v0.9.0"; exit 1; fi
	@echo "+ Tagging and pushing tag: $(VERSION)"
	@git tag -a $(VERSION) -m "Gorums $(VERSION)"
	@git push origin $(VERSION)
	@echo "+ Tag pushed. Use 'gh release create $(VERSION) --title \"Gorums $(VERSION)\"' to publish with release notes."
