CMD_PKG 			:= cmd
PLUGINS_PKG 			:= plugins
BUNDLE_PKG			:= bundle
DEV_PKG				:= dev
PLUGIN_PKG 			:= gorums

GORUMS_PKGS 			:= $(shell go list ./... | grep -v /vendor/)
GORUMS_FILES			:= $(shell find . -name '*.go' -not -path "*vendor*")
GORUMS_DIRS 			:= $(shell find . -type d -not -path "*vendor*" -not -path "./.git*" -not -path "*testdata*")
GORUMS_PKG_PATH	 		:= github.com/relab/gorums
GORUMS_DEV_PKG_PATH		:= $(GORUMS_PKG_PATH)/$(DEV_PKG)
GORUMS_ENV_GENDEV		:= GORUMSGENDEV=1

GORUMS_STATIC_GO		:= $(PLUGINS_PKG)/$(PLUGIN_PKG)/static.go
BUNDLE_MAIN_GO 			:= $(CMD_PKG)/$(BUNDLE_PKG)/main.go
GENTEMPLATES_MAIN_GO 		:= $(CMD_PKG)/gentemplates/main.go

PROTOC_PLUGIN_PKG		:= protoc-gen-gorums
PROTOC_PLUGIN_PKG_PATH 		:= $(GORUMS_PKG_PATH)/$(CMD_PKG)/$(PROTOC_PLUGIN_PKG)
PROTOC_PLUGIN_NAME 		:= gorums_out

TESTDATA_REG			:= testdata/register_golden

REG_PROTO_NAME			:= register.proto
REG_PBGO_NAME			:= register.pb.go
REG_PROTO_DEV_RPATH		:= $(DEV_PKG)/$(REG_PROTO_NAME)
REG_PROTO_TEST_RPATH		:= $(TESTDATA_REG)/$(REG_PROTO_NAME)
REG_PBGO_TEST_RPATH		:= $(TESTDATA_REG)/$(REG_PBGO_NAME)

.PHONY: all
all: build test check

.PHONY: build
build:
	@echo go build:
	@go build -v -i $(GORUMS_PKGS)

.PHONY: test
test: reinstallprotoc
	@echo go test:
	@go test -v $(GORUMS_PKGS)

.PHONY: testrace
testrace: reinstallprotoc
	@echo go test -race:
	@go test -v -race -cpu=1,2,4 $(GORUMS_PKGS)

.PHONY: benchlocal
benchlocal:
	go test -v $(GORUMS_DEV_PKG_PATH) -run=^$$ -bench=Local$$ -benchtime=5s

.PHONY: benchremote
benchremote:
	go test -v $(GORUMS_DEV_PKG_PATH) -run=^$$ -bench=Remote$$ -benchtime=5s

.PHONY: clean
clean:
	go clean -i $(GORUMS_PKG_PATH)/...
	find . -name '*.test' -type f -exec rm -f {} \;
	find . -name '*.prof' -type f -exec rm -f {} \;

.PHONY: reinstallprotoc
reinstallprotoc:
	@echo installing protoc-gen-gorums with gorums plugin linked
	@go install $(PROTOC_PLUGIN_PKG_PATH)

.PHONY: gendevproto
gendevproto: reinstallprotoc
	@echo generating gorumsdev register proto
	@protoc --$(PROTOC_PLUGIN_NAME)=plugins=grpc:. $(REG_PROTO_DEV_RPATH)

.PHONY: genstatic
genstatic:
	@echo creating static gorums plugin code bundle
	@go run $(BUNDLE_MAIN_GO) $(GORUMS_DEV_PKG_PATH) > $(GORUMS_STATIC_GO)

.PHONY: gentemplates
gentemplates:
	@echo creating templates for gorums plugin code bundle
	@go run $(GENTEMPLATES_MAIN_GO)

.PHONY: gengolden
gengolden: genstatic gentemplates reinstallprotoc
	@echo generating golden output
	@cp $(REG_PROTO_DEV_RPATH) $(REG_PROTO_TEST_RPATH)
	@protoc --$(PROTOC_PLUGIN_NAME)=plugins=grpc+gorums:. $(REG_PROTO_TEST_RPATH)

.PHONY: gendev
gendev: genstatic gentemplates reinstallprotoc
	@echo generating _gen.go files for dev
	@cp $(REG_PROTO_DEV_RPATH) $(REG_PROTO_TEST_RPATH)
	@$(GORUMS_ENV_GENDEV) protoc --$(PROTOC_PLUGIN_NAME)=plugins=grpc+gorums:. $(REG_PROTO_TEST_RPATH)
	@git checkout $(REG_PBGO_TEST_RPATH)

.PHONY: gengoldenanddev
gengoldenanddev: genstatic gentemplates reinstallprotoc
	@echo generating golden output and _gen.go files for dev
	@cp $(REG_PROTO_DEV_RPATH) $(REG_PROTO_TEST_RPATH)
	@$(GORUMS_ENV_GENDEV) protoc --$(PROTOC_PLUGIN_NAME)=plugins=grpc+gorums:. $(REG_PROTO_TEST_RPATH)

.PHONY: profcpu
profcpu:
	go test $(GORUMS_DEV_PKG_PATH) -run=NONE -bench=RegisterRead -cpuprofile cpu.prof
	go tool pprof $(DEV_PKG).test cpu.prof

.PHONY: profmem
profmem:
	go test $(GORUMS_DEV_PKG_PATH) -run=NONE -bench=RegisterRead -memprofile allocmem.prof
	go tool pprof -alloc_space $(DEV_PKG).test allocmem.prof

.PHONY: profobj
profobj:
	go test $(GORUMS_DEV_PKG_PATH) -run=NONE -bench=RegisterRead -memprofile allocobj.prof
	go tool pprof -alloc_objects $(DEV_PKG).test allocobj.prof

.PHONY: getgvt
getgvt:
	go get -u github.com/FiloSottile/gvt

.PHONY: getchecktools
getchecktools:
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/golang/lint/golint
	go get -u github.com/jgautheron/goconst/cmd/goconst
	go get -u github.com/kisielk/errcheck
	go get -u github.com/gordonklaus/ineffassign
	go get -u github.com/mdempsky/unconvert
	go get -u honnef.co/go/unused/cmd/unused
	go get -u honnef.co/go/simple/cmd/gosimple
	go get -u honnef.co/go/staticcheck/cmd/staticcheck
	go get -u github.com/mvdan/interfacer/cmd/interfacer
	go get -u github.com/client9/misspell/cmd/misspell

.PHONY: check
check:
	@echo static analysis tools:
	@echo "gofmt (simplify)"
	@! gofmt -s -l $(GORUMS_FILES) | grep -vF 'No Exceptions'
	@echo "goimports"
	@! goimports -l $(GORUMS_FILES) | grep -vF 'No Exceptions'
	@echo "vet"
	@! go tool vet $(GORUMS_DIRS) 2>&1 | \
		grep -vF 'vendor/' | \
		grep -vE '^dev/config_rpc_test.go:.+: constant [0-9]+ not a string in call to Errorf'
	@echo "vet --shadow"
	@! go tool vet --shadow $(GORUMS_DIRS) 2>&1 | grep -vF 'vendor/'
	@echo "golint"
	@for pkg in $(GORUMS_PKGS); do \
		! golint $$pkg | \
		grep -vE '(\.pb\.go)' | \
		grep -vE 'gorums/plugins/gorums/templates.go' ; \
	done
	@echo "goconst"
	@for dir in $(GORUMS_DIRS); do \
		! goconst $$dir | \
		grep -vE '("_"|.pb.go)' ; \
	done
	@echo "errcheck"
	@errcheck -ignore 'bytes:WriteString,encoding/binary:Write,os:Close|Remove*,net:Close,github.com/relab/gorums/dev:Close' $(GORUMS_PKGS)
	@echo "ineffassign"
	@for dir in $(GORUMS_DIRS); do \
		ineffassign -n $$dir ; \
	done
	@echo "unconvert"
	@! unconvert $(GORUMS_PKGS) | grep -vF '.pb.go'
	@echo "unused"
	@unused $(GORUMS_PKGS)
	@echo "gosimple"
	@for pkg in $(GORUMS_PKGS); do \
		! gosimple $$pkg | \
		grep -vF 'pb.go' ; \
	done
	@echo "interfacer"
	@interfacer $(GORUMS_PKGS)
	@echo "missspell"
	@! misspell ./**/* | grep -vF '/vendor/'
	@echo "staticcheck"
	@staticcheck $(GORUMS_PKGS)

.PHONY: updatedeps 
updatedeps:
	gvt update --all
