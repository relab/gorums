//go:build tools
// +build tools

package gorums

import (
	_ "golang.org/x/tools/cmd/stress"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
