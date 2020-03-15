PLUGIN_PATH				:= cmd/protoc-gen-gorums
PLUGIN_DEV_PATH			:= $(PLUGIN_PATH)/dev
PLUGIN_INTERNAL_PATH	:= $(PLUGIN_PATH)/internalgorums
PLUGIN_TESTS_PATH		:= $(PLUGIN_PATH)/tests
PLUGIN_STATIC_FILE		:= $(PLUGIN_INTERNAL_PATH)/template_static.go

# TODO clean up the make targets to include relevant dependent src files

# TODO evaluate if these should be PHONY (dev and devsingle shouldn't, but need to make dependency handling)
.PHONY: clean download install-tools dev devsingle

clean:
	rm -f $(PLUGIN_STATIC_FILE).bak

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

gorumsproto: install-tools
	@echo Generating gorums proto options
	@protoc --go_out=paths=source_relative:. gorums.proto

installgorums: gorumsproto
	@echo Installing protoc-gen-gorums compiler plugin for protoc
	@go install github.com/relab/gorums/cmd/protoc-gen-gorums

bundle: installgorums
	cp $(PLUGIN_STATIC_FILE) $(PLUGIN_STATIC_FILE).bak
	protoc -I$(PLUGIN_DEV_PATH):. --gorums_out=bundle=$(PLUGIN_STATIC_FILE):. $(PLUGIN_DEV_PATH)/zorums.proto

dev: installgorums
	@echo Generating Gorums code for zorums.proto as a multiple _gorums.pb.go files in dev folder
	rm -f $(PLUGIN_DEV_PATH)/*.pb.go
	protoc -I$(PLUGIN_DEV_PATH):. \
		--go_out=:. \
		--go-grpc_out=:. \
		--gorums_out=dev=true,trace=true:. \
		$(PLUGIN_DEV_PATH)/zorums.proto

devsingle: installgorums
	@echo Generating Gorums code for zorums.proto as a single _gorums.pb.go file in dev folder
	rm -f $(PLUGIN_DEV_PATH)/*.pb.go
	protoc -I$(PLUGIN_DEV_PATH):. \
		--go_out=:. \
		--go-grpc_out=:. \
		--gorums_out=trace=true:. \
		$(PLUGIN_DEV_PATH)/zorums.proto

# TODO(meling) make test case comparing generated code with old code

.PHONY: qc
qc: installgorums
	protoc -I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--gorums_out=paths=source_relative,trace=true:. \
		$(PLUGIN_TESTS_PATH)/quorumcall/quorumcall.proto
