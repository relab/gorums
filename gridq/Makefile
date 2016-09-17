PROTOC_PLUGIN 			:= gorums_out

.PHONY: installprotocgorums
installprotocgorums:
	@echo installing protoc-gen-gorums with gorums linked...
	@go install github.com/relab/gorums/cmd/protoc-gen-gorums

.PHONY: proto
proto: installprotocgorums
	protoc --$(PROTOC_PLUGIN)=plugins=grpc+gorums:. gridq.proto

.PHONY: bench 
bench:
	go test github.com/relab/gridq/cmd/gqclient -run=NONE -benchmem -benchtime=5s -bench=.
