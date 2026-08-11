// Package dev is the development and testing scaffold for protoc-gen-gorums.
//
// It holds the static support code and the generated example output
// (the zorums_*.pb.go files, regenerated from zorums.proto by `make dev`) used
// to develop and test the plugin. The plugin bundler discovers the identifiers
// declared here (see aliases.go) and adds them to the reserved-identifier list
// baked into the generator.
package dev
