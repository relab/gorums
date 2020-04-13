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
	"io":             protogen.GoImportPath("io"),
	"time":           protogen.GoImportPath("time"),
	"fmt":            protogen.GoImportPath("fmt"),
	"log":            protogen.GoImportPath("log"),
	"sync":           protogen.GoImportPath("sync"),
	"atomic":         protogen.GoImportPath("sync/atomic"),
	"context":        protogen.GoImportPath("context"),
	"trace":          protogen.GoImportPath("golang.org/x/net/trace"),
	"grpc":           protogen.GoImportPath("google.golang.org/grpc"),
	"codes":          protogen.GoImportPath("google.golang.org/grpc/codes"),
	"status":         protogen.GoImportPath("google.golang.org/grpc/status"),
	"gorums":         protogen.GoImportPath("github.com/relab/gorums"),
	"rand":           protogen.GoImportPath("math/rand"),
	"backoff":        protogen.GoImportPath("google.golang.org/grpc/backoff"),
	"math":           protogen.GoImportPath("math"),
	"ptypes":         protogen.GoImportPath("github.com/golang/protobuf/ptypes"),
	"strictordering": protogen.GoImportPath("github.com/relab/gorums/strictordering"),
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
	"withQFArg": func(method *protogen.Method, arg string) string {
		if hasMethodOption(method, gorums.E_QfWithReq) {
			return arg
		}
		return ""
	},
	"correctableStream": func(method *protogen.Method) bool {
		return hasMethodOption(method, gorums.E_CorrectableStream)
	},
	"withCorrectable": func(method *protogen.Method, arg string) string {
		if hasMethodOption(method, gorums.E_Correctable, gorums.E_CorrectableStream) {
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
	"fullName": func(method *protogen.Method) string {
		return fmt.Sprintf("/%s/%s", method.Parent.Desc.FullName(), method.Desc.Name())
	},
	"serviceName": func(method *protogen.Method) string {
		return fmt.Sprintf("%s", method.Parent.Desc.Name())
	},
	"in": func(g *protogen.GeneratedFile, method *protogen.Method) string {
		return g.QualifiedGoIdent(method.Input.GoIdent)
	},
	"out":                       out,
	"internalOut":               internalOut,
	"customOut":                 customOut,
	"futureOut":                 futureOut,
	"correctableOut":            correctableOut,
	"correctableStreamOut":      correctableStreamOut,
	"mapInternalOutType":        mapInternalOutType,
	"mapCorrectableOutType":     mapCorrectableOutType,
	"mapFutureOutType":          mapFutureOutType,
	"streamMethods":             streamMethods,
	"callTypeName":              callTypeName,
	"qspecServices":             qspecServices,
	"qspecMethods":              qspecMethods,
	"unexport":                  unexport,
	"qcresult":                  qcresult,
	"contains":                  strings.Contains,
	"field":                     field,
	"hasStrictOrderingMethods":  hasStrictOrderingMethods,
	"exclusivelyStrictOrdering": exclusivelyStrictOrdering,
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

func internal(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
	if hasMethodOption(method, callTypesWithInternal...) {
		out := out(g, method)
		intOut := internalOut(out)
		s[intOut] = out
	}
}

func internalOut(out string) string {
	return fmt.Sprintf("internal%s", field(out))
}

func future(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
	if hasMethodOption(method, gorums.E_QcFuture) {
		out := customOut(g, method)
		futOut := futureOut(out)
		s[futOut] = out
	}
}

func futureOut(customOut string) string {
	return fmt.Sprintf("Future%s", field(customOut))
}

func correctable(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
	if hasMethodOption(method, gorums.E_Correctable) {
		out := customOut(g, method)
		corrOut := correctableOut(out)
		s[corrOut] = out
	}
	if hasMethodOption(method, gorums.E_CorrectableStream) {
		out := customOut(g, method)
		corrOut := correctableStreamOut(out)
		s[corrOut] = out
	}
}

func correctableOut(customOut string) string {
	return fmt.Sprintf("Correctable%s", field(customOut))
}

func correctableStreamOut(customOut string) string {
	return fmt.Sprintf("CorrectableStream%s", field(customOut))
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

func hasStrictOrderingMethods(services []*protogen.Service) bool {
	for _, service := range services {
		for _, method := range service.Methods {
			if hasMethodOption(method, gorums.E_Ordered) {
				return true
			}
		}
	}
	return false
}

func exclusivelyStrictOrdering(service *protogen.Service) bool {
	for _, method := range service.Methods {
		if !hasMethodOption(method, gorums.E_Ordered) {
			return false
		}
	}
	return true
}
