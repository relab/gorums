package gengorums

import (
	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

// GenerateDevFiles generates a zorums_{{gorumsType}}_gorums.pb.go file for each Gorums datatype
// and for each call type in the service definition.
func GenerateDevFiles(gen *protogen.Plugin, file *protogen.File) {
	if !gorumsGuard(file) {
		return
	}
	for gorumsType := range gorumsCallTypesInfo {
		generateDevFile(gen, file, gorumsType)
	}
}

func generateDevFile(gen *protogen.Plugin, file *protogen.File, gorumsType string) {
	// generate dev file for given gorumsType
	filename := file.GeneratedFilenamePrefix + "_" + gorumsType + "_gorums.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	genGeneratedHeader(gen, file, g)
	g.P("package ", file.GoPackageName)
	g.P()
	genVersionCheck(g)
	if gorumsType == callTypeName(gorums.E_Multicast) || gorumsType == callTypeName(gorums.E_Unicast) {
		genReferenceImports(g, file.Services)
	}
	genGorumsType(g, file.Services, gorumsType)
	genGorumsMethods(g, file.Services, gorumsType)
}
