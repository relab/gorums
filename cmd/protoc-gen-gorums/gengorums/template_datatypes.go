package gengorums

import (
	"fmt"
	"strings"

	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

var internalOutDataType = `
{{range $out, $intOut := mapInternalOutType .GenFile .Services}}
type {{$intOut}} struct {
	nid   uint32
	reply *{{$out}}
	err   error
}
{{end}}
`

var futureDataType = `
{{range $customOut, $futureOut := mapFutureOutType .GenFile .Services}}
// {{$futureOut}} is a future object for processing replies.
type {{$futureOut}} struct {
	// the actual reply
	*{{$customOut}}
	NodeIDs  []uint32
	err      error
	c        chan struct{}
}
{{end}}
`

var correctableDataType = `
{{$genFile := .GenFile}}
{{range $customOut, $correctableOut := mapCorrectableOutType .GenFile .Services}}
// {{$correctableOut}} is a correctable object for processing replies.
type {{$correctableOut}} struct {
	mu {{use "sync.Mutex" $genFile}}
	// the actual reply
	*{{$customOut}}
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level	int
		ch		chan struct{}
	}
	donech chan struct{}
}
{{end}}
`

var datatypes = internalOutDataType +
	futureDataType +
	correctableDataType

type mapFunc func(*protogen.GeneratedFile, *protogen.Method, map[string]string)

// mapType returns a map of types as defined by the function mapFn.
func mapType(g *protogen.GeneratedFile, services []*protogen.Service, mapFn mapFunc) (s map[string]string) {
	s = make(map[string]string)
	for _, service := range services {
		for _, method := range service.Methods {
			mapFn(g, method, s)
		}
	}
	return s
}

func out(g *protogen.GeneratedFile, method *protogen.Method) string {
	return g.QualifiedGoIdent(method.Output.GoIdent)
}

func internal(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
	if hasMethodOption(method, callTypesWithInternal...) {
		out := out(g, method)
		s[out] = internalOut(g, method)
	}
}

func internalOut(g *protogen.GeneratedFile, method *protogen.Method) string {
	out := g.QualifiedGoIdent(method.Output.GoIdent)
	return fmt.Sprintf("internal%s", out[strings.LastIndex(out, ".")+1:])
}

func future(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
	if hasMethodOption(method, gorums.E_QcFuture) {
		out := customOut(g, method)
		s[out] = futureOut(g, method)
	}
}

func futureOut(g *protogen.GeneratedFile, method *protogen.Method) string {
	out := g.QualifiedGoIdent(method.Output.GoIdent)
	return fmt.Sprintf("Future%s", out[strings.LastIndex(out, ".")+1:])
}

// field derives an embedded field name from the given typeName.
// If typeName contains a package, this will be removed.
func field(typeName string) string {
	return typeName[strings.LastIndex(typeName, ".")+1:]
}

func customOut(g *protogen.GeneratedFile, method *protogen.Method) string {
	ext := protoimpl.X.MessageOf(method.Desc.Options()).Interface()
	customOutType := fmt.Sprintf("%v", proto.GetExtension(ext, gorums.E_CustomReturnType))
	outType := method.Output.GoIdent
	if customOutType != "" {
		outType.GoName = customOutType
	}
	return g.QualifiedGoIdent(outType)
}

func correctable(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
	//TODO fix stream version; not clear if it needs a separate mapping function
	if hasMethodOption(method, gorums.E_Correctable) {
		//TODO(meling) fix customOut for Correctable
		out := customOut(g, method)
		s[out] = fmt.Sprintf("Correctable%s", method.Output.GoIdent.GoName)
	}
	if hasMethodOption(method, gorums.E_CorrectableStream) {
		//TODO(meling) fix customOut for CorrectableStream
		out := customOut(g, method)
		s[out] = fmt.Sprintf("CorrectableStream%s", method.Output.GoIdent.GoName)
	}
}

func mapInternalOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, internal)
}

func mapFutureOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, future)
}

func mapCorrectableOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, correctable)
}
