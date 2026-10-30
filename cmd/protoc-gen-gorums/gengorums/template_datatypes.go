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

// quorumCallMethods returns the quorum call methods of the given service;
// each one produces the Correctable alias of its response type.
func quorumCallMethods(service *protogen.Service) []*protogen.Method {
	var methods []*protogen.Method
	for _, method := range service.Methods {
		if hasMethodOption(method, gorums.E_Quorumcall) {
			methods = append(methods, method)
		}
	}
	return methods
}

// The Correctable aliases are generated once per Go package; .Aliases holds
// those that this file declares.
var correctableDataType = `
{{$correctable := use "gorums.Correctable" .GenFile}}
{{range $alias, $method := .Aliases}}
{{- $out := out $.GenFile $method}}
// {{$alias}} is a correctable object for quorum calls returning *{{$out}}.
type {{$alias}} = *{{$correctable}}[*{{$out}}]
{{end}}
`

var dataTypes = correctableDataType
