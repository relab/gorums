package gengorums

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

// importMap holds the mapping between short-hand import name
// and full import path for the default package.
var importMap = map[string]protogen.GoImportPath{
	"io":           protogen.GoImportPath("io"),
	"time":         protogen.GoImportPath("time"),
	"fmt":          protogen.GoImportPath("fmt"),
	"log":          protogen.GoImportPath("log"),
	"math":         protogen.GoImportPath("math"),
	"rand":         protogen.GoImportPath("math/rand"),
	"sync":         protogen.GoImportPath("sync"),
	"atomic":       protogen.GoImportPath("sync/atomic"),
	"context":      protogen.GoImportPath("context"),
	"trace":        protogen.GoImportPath("golang.org/x/net/trace"),
	"grpc":         protogen.GoImportPath("google.golang.org/grpc"),
	"codes":        protogen.GoImportPath("google.golang.org/grpc/codes"),
	"status":       protogen.GoImportPath("google.golang.org/grpc/status"),
	"backoff":      protogen.GoImportPath("google.golang.org/grpc/backoff"),
	"proto":        protogen.GoImportPath("google.golang.org/protobuf/proto"),
	"gorums":       protogen.GoImportPath("github.com/relab/gorums"),
	"ordering":     protogen.GoImportPath("github.com/relab/gorums/ordering"),
	"protoreflect": protogen.GoImportPath("google.golang.org/protobuf/reflect/protoreflect"),
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
		path, ident, _ := strings.Cut(pkgIdent, ".")
		pkg, ok := importMap[path]
		if !ok {
			return "IMPORT NOT FOUND: " + path
		}
		return g.QualifiedGoIdent(pkg.Ident(ident))
	},
	"correctableStream": func(method *protogen.Method) bool {
		return hasMethodOption(method, gorums.E_Correctable) && method.Desc.IsStreamingServer()
	},
	"isCorrectable": func(method *protogen.Method) bool {
		return hasMethodOption(method, gorums.E_Correctable)
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
		return string(method.Parent.Desc.Name())
	},
	"in": func(g *protogen.GeneratedFile, method *protogen.Method) string {
		return g.QualifiedGoIdent(method.Input.GoIdent)
	},
	"isOneway": func(method *protogen.Method) bool {
		return hasMethodOption(method, gorums.E_Multicast, gorums.E_Unicast)
	},
	"out":                   out,
	"outType":               outType,
	"mapCorrectableOutType": mapCorrectableOutType,
	"mapAsyncOutType":       mapAsyncOutType,
	"unexport":              unexport,
	"contains":              strings.Contains,
	"field":                 field,
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

func mapAsyncOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, func(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
		// Generate Async type aliases for quorumcall methods since users can
		// call .AsyncMajority() on any quorum call result
		if hasMethodOption(method, gorums.E_Quorumcall) {
			o := out(g, method)
			futOut := fmt.Sprintf("Async%s", field(o))
			s[futOut] = o
		}
	})
}

func mapCorrectableOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, func(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
		if hasMethodOption(method, gorums.E_Correctable) {
			o := out(g, method)
			corrOut := outType(method, o)
			s[corrOut] = o
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
	return template.Must(template.New(name).Funcs(funcMap).Parse(tmpl))
}

func mustExecute(t *template.Template, data any) string {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}
