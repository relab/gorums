//go:build tools
// +build tools

package examples

import (
	_ "github.com/relab/gorums/cmd/protoc-gen-gorums"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
