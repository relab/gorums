package gengorums

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

// importMap holds the mapping between short-hand import name
// and full import path for the default package.
var importMap = map[string]protogen.GoImportPath{
	"io":       protogen.GoImportPath("io"),
	"time":     protogen.GoImportPath("time"),
	"fmt":      protogen.GoImportPath("fmt"),
	"log":      protogen.GoImportPath("log"),
	"sync":     protogen.GoImportPath("sync"),
	"atomic":   protogen.GoImportPath("sync/atomic"),
	"context":  protogen.GoImportPath("context"),
	"trace":    protogen.GoImportPath("golang.org/x/net/trace"),
	"grpc":     protogen.GoImportPath("google.golang.org/grpc"),
	"codes":    protogen.GoImportPath("google.golang.org/grpc/codes"),
	"status":   protogen.GoImportPath("google.golang.org/grpc/status"),
	"gorums":   protogen.GoImportPath("github.com/relab/gorums"),
	"rand":     protogen.GoImportPath("math/rand"),
	"backoff":  protogen.GoImportPath("google.golang.org/grpc/backoff"),
	"math":     protogen.GoImportPath("math"),
	"ordering": protogen.GoImportPath("github.com/relab/gorums/ordering"),
	"proto":    protogen.GoImportPath("google.golang.org/protobuf/proto"),
}

func addImport(path, ident string, g *protogen.GeneratedFile) string {
	pkg := path[strings.LastIndex(path, "/")+1:]
	impPath, ok := importMap[pkg]
	if !ok {
		impPath = protogen.GoImportPath(path)
		importMap[pkg] = impPath
	}
	return g.QualifiedGoIdent(impPath.Ident(ident))
}

var funcMap = template.FuncMap{
	// this function will stop the generator if incorrect input is used
	// the output contains the descriptive strings below to help debug any bad inputs.
	"use": func(pkgIdent string, g *protogen.GeneratedFile) string {
		if strings.Count(pkgIdent, ".") != 1 {
			return "EXPECTED PACKAGE NAME AND IDENTIFIER, but got: " + pkgIdent
		}
		i := strings.Index(pkgIdent, ".")
		path, ident := pkgIdent[0:i], pkgIdent[i+1:]
		pkg, ok := importMap[path]
		if !ok {
			return "IMPORT NOT FOUND: " + path
		}
		return g.QualifiedGoIdent(pkg.Ident(ident))
	},
	"hasPerNodeArg": func(method *protogen.Method) bool {
		return hasMethodOption(method, gorums.E_PerNodeArg)
	},
	"perNodeArg": func(method *protogen.Method, arg string) string {
		if hasMethodOption(method, gorums.E_PerNodeArg) {
			return arg
		}
		return ""
	},
	"perNodeFnType": func(g *protogen.GeneratedFile, method *protogen.Method, arg string) string {
		if hasMethodOption(method, gorums.E_PerNodeArg) {
			inType := g.QualifiedGoIdent(method.Input.GoIdent)
			return arg + " func(*" + inType + ", uint32) *" + inType
		}
		return ""
	},
	"correctableStream": func(method *protogen.Method) bool {
		return hasMethodOption(method, gorums.E_Correctable) && method.Desc.IsStreamingServer()
	},
	"withCorrectable": func(method *protogen.Method, arg string) string {
		if hasMethodOption(method, gorums.E_Correctable) {
			return arg
		}
		return ""
	},
	"withPromise": func(method *protogen.Method, arg string) string {
		if hasMethodOption(method, callTypesWithPromiseObject...) {
			return arg
		}
		return ""
	},
	"docName": func(method *protogen.Method) string {
		return callType(method).docName
	},
	"fullName": func(method *protogen.Method) string {
		return fmt.Sprintf("/%s/%s", method.Parent.Desc.FullName(), method.Desc.Name())
	},
	"serviceName": func(method *protogen.Method) string {
		return fmt.Sprintf("%s", method.Parent.Desc.Name())
	},
	"in": func(g *protogen.GeneratedFile, method *protogen.Method) string {
		return g.QualifiedGoIdent(method.Input.GoIdent)
	},
	"out":                   out,
	"outType":               outType,
	"internalOut":           internalOut,
	"customOut":             customOut,
	"mapInternalOutType":    mapInternalOutType,
	"mapCorrectableOutType": mapCorrectableOutType,
	"mapFutureOutType":      mapFutureOutType,
	"streamMethods":         streamMethods,
	"qspecServices":         qspecServices,
	"qspecMethods":          qspecMethods,
	"orderedMethods":        orderedMethods,
	"unexport":              unexport,
	"qcresult":              qcresult,
	"contains":              strings.Contains,
	"field":                 field,
	"hasOrderingMethods":    hasOrderingMethods,
	"exclusivelyOrdering":   exclusivelyOrdering,
}

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

func outType(method *protogen.Method, out string) string {
	return fmt.Sprintf("%s%s", callType(method).outPrefix, field(out))
}

func internalOut(out string) string {
	return fmt.Sprintf("internal%s", field(out))
}

// customOut returns the output type to be used for the given method.
// This may be the output type specified in the rpc line,
// or if a custom_return_type option is provided for the method,
// this provided custom type will be returned.
func customOut(g *protogen.GeneratedFile, method *protogen.Method) string {
	ext := protoimpl.X.MessageOf(method.Desc.Options()).Interface()
	customOutType := fmt.Sprintf("%v", proto.GetExtension(ext, gorums.E_CustomReturnType))
	outType := method.Output.GoIdent
	if customOutType != "" {
		outType.GoName = customOutType
	}
	return g.QualifiedGoIdent(outType)
}

func mapInternalOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, func(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
		if hasMethodOption(method, callTypesWithInternal...) {
			out := out(g, method)
			intOut := internalOut(out)
			s[intOut] = out
		}
	})
}

func mapFutureOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, func(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
		if hasMethodOption(method, gorums.E_QcFuture) {
			out := customOut(g, method)
			futOut := outType(method, out)
			s[futOut] = out
		}
	})
}

func mapCorrectableOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, func(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
		if hasMethodOption(method, gorums.E_Correctable) {
			out := customOut(g, method)
			corrOut := outType(method, out)
			s[corrOut] = out
		}
	})
}

// field derives an embedded field name from the given typeName.
// If typeName contains a package, this will be removed.
func field(typeName string) string {
	return typeName[strings.LastIndex(typeName, ".")+1:]
}

func unexport(s string) string { return strings.ToLower(s[:1]) + s[1:] }

func parseTemplate(name, tmpl string) *template.Template {
	return template.Must(template.New(name).Funcs(funcMap).Parse(trace() + tmpl))
}

func mustExecute(t *template.Template, data interface{}) string {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}

func hasOrderingMethods(services []*protogen.Service) bool {
	for _, service := range services {
		for _, method := range service.Methods {
			if hasMethodOption(method, gorums.E_Ordered) {
				return true
			}
		}
	}
	return false
}

func exclusivelyOrdering(service *protogen.Service) bool {
	for _, method := range service.Methods {
		if !hasMethodOption(method, gorums.E_Ordered) {
			return false
		}
	}
	return true
}
