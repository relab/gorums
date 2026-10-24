package gengorums

import (
	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

// correctableAlias returns the name of the Correctable alias for the output
// type of the quorum call method.
func correctableAlias(method *protogen.Method) string {
	return "Correctable" + method.Output.GoIdent.GoName
}

// typeAliasIdents returns the names of the Correctable aliases that the
// quorum calls of the given service produce. Services of one Go package share
// these aliases; [dataTypes] declares each one once per Go package.
func typeAliasIdents(service *protogen.Service) []string {
	var idents []string
	for _, method := range service.Methods {
		if hasMethodOption(method, gorums.E_Quorumcall) {
			idents = append(idents, correctableAlias(method))
		}
	}
	return idents
}

// This type alias is generated only once per return type for a correctable
// call type. That is, if multiple correctable calls use the same return type,
// this type alias is only generated once. An alias in .Declared belongs to an
// earlier file of the Go package and is not generated again.
var correctableDataType = `
{{$correctable := use "gorums.Correctable" .GenFile}}
{{range $correctableOut, $customOut := mapCorrectableOutType .GenFile .Services .Declared}}
// {{$correctableOut}} is a correctable object for quorum calls returning *{{$customOut}}.
type {{$correctableOut}} = *{{$correctable}}[*{{$customOut}}]
{{end}}
`

var dataTypes = correctableDataType
