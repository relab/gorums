package gengorums

import (
	"google.golang.org/protobuf/compiler/protogen"
)

// GenerateDevFiles generates a zorums_{{gorumsType}}_gorums.pb.go file for each Gorums datatype
// and for each call type in the service definition.
func GenerateDevFiles(gen *protogen.Plugin, file *protogen.File) {
	if !gorumsGuard(file) {
		return
	}

	initReservedNames()

	for gorumsType := range gorumsCallTypesInfo {
		generateDevFile(gen, file, gorumsType)
	}

	checkNameCollision(file)
}

func generateDevFile(gen *protogen.Plugin, file *protogen.File, gorumsType string) {
	// generate dev file for given gorumsType
	filename := file.GeneratedFilenamePrefix + "_" + gorumsType + "_gorums.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	genGeneratedHeader(gen, g, file)
	g.P("package ", file.GoPackageName)
	g.P()
	genVersionCheck(g)
	genGorumsType(g, file.Services, gorumsType)
}
