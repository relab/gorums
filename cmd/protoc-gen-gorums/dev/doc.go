// Package dev is the development and testing scaffold for protoc-gen-gorums.
//
// It holds the static support code and the generated example output
// (the zorums_*.pb.go files, regenerated from zorums.proto by `make dev`) used
// to develop and test the plugin. The plugin bundler copies the declarations
// of the other non-test Go files here (the static code, see aliases.go) into
// the generator.
//
// The generator emits the static code in one generated file per Go package
// and protoc run, so the static code may declare package-level names. Each
// exported name becomes a reserved identifier: a proto message or enum of the
// Go package must not take it. All proto files of a Go package must be
// compiled in one protoc run; otherwise, two generated files declare the
// static names.
package dev
