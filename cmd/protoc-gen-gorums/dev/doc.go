// Package dev is the development and testing scaffold for protoc-gen-gorums.
//
// It holds the static support code and the generated example output
// (the zorums_*.pb.go files, regenerated from zorums.proto by `make dev`) used
// to develop and test the plugin. The plugin bundler copies the declarations
// of the other non-test Go files here (the static code, see aliases.go) into
// the generator.
//
// The generator emits the static code into the gorums.pb.go file of each Go
// package, so the static code may declare package-level names. Every protoc
// run that generates Gorums code for the package writes the same gorums.pb.go
// file. Each exported name becomes a reserved identifier: a proto message,
// enum, or enum value of the Go package must not take it. In dev mode, the
// generator writes no gorums.pb.go file; aliases.go declares the names.
package dev
