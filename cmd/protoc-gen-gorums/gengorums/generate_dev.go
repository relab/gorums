package gengorums

import (
	"maps"
	"slices"

	"google.golang.org/protobuf/compiler/protogen"
)

// File names of the dev files that do not hold the methods of a call type.
const (
	devClientName = "client"
	devServerName = "server"
)

// generateDevFiles generates a separate {prefix}_{name}_gorums.pb.go file for
// each part of the Gorums code of file, where prefix is the generated file name
// prefix of file. The parts are the client interfaces, the server interface,
// and the client methods of each call type in [gorumsCallTypesInfo], named by
// the call type.
func generateDevFiles(gen *protogen.Plugin, file *protogen.File) {
	g := newDevFile(gen, file, devClientName)
	genReferenceImports(g, file.Services)
	genClients(g, file.Services)

	for _, name := range slices.Sorted(maps.Keys(gorumsCallTypesInfo)) {
		g := newDevFile(gen, file, name)
		if gorumsCallTypesInfo[name].oneway {
			genReferenceImports(g, file.Services)
		}
		genMethods(g, file, name)
	}

	genServer(newDevFile(gen, file, devServerName), file.Services)
}

// newDevFile returns the generated dev file with the given name.
func newDevFile(gen *protogen.Plugin, file *protogen.File, name string) *protogen.GeneratedFile {
	return newGorumsFile(gen, file, "_"+name+"_gorums.pb.go")
}
