package dev

import gorums "github.com/relab/gorums"

// The type aliases below are useful Gorums types that we make accessible
// from generated code. These names therefore become reserved identifiers,
// meaning that proto message types with these names would collide with the
// generated aliases and cause a compile error.
//
// The bundler (gorums_bundle.go) is responsible for discovering these
// aliases and any other identifiers defined herein, and adding them to
// the reserved identifiers list.
//
// If necessary, additional aliases and other identifiers should be added in
// the generator's cmd/protoc-gen-gorums/dev directory, and the bundler will
// automatically discover them and add them to the reserved identifiers list.

type (
	Configuration = gorums.Configuration
	Node          = gorums.Node
	NodeContext   = gorums.NodeContext
	ConfigContext = gorums.ConfigContext
)
