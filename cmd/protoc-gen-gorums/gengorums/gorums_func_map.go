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
	"io":      protogen.GoImportPath("io"),
	"time":    protogen.GoImportPath("time"),
	"fmt":     protogen.GoImportPath("fmt"),
	"log":     protogen.GoImportPath("log"),
	"sync":    protogen.GoImportPath("sync"),
	"context": protogen.GoImportPath("context"),
	"trace":   protogen.GoImportPath("golang.org/x/net/trace"),
	"grpc":    protogen.GoImportPath("google.golang.org/grpc"),
	"codes":   protogen.GoImportPath("google.golang.org/grpc/codes"),
	"status":  protogen.GoImportPath("google.golang.org/grpc/status"),
	"gorums":  protogen.GoImportPath("github.com/relab/gorums"),
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
	"in": func(g *protogen.GeneratedFile, method *protogen.Method) string {
		return g.QualifiedGoIdent(method.Input.GoIdent)
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
			return arg +
				" func(*" + g.QualifiedGoIdent(method.Input.GoIdent) +
				", uint32) *" + g.QualifiedGoIdent(method.Input.GoIdent)
		}
		return ""
	},
	"out":         out,
	"internalOut": internalOut,
	"customOut":   customOut,
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
	"callTypeName": callTypeName,
	"fullName": func(method *protogen.Method) string {
		return fmt.Sprintf("/%s/%s", method.Parent.Desc.FullName(), method.Desc.Name())
	},
	"serviceName": func(method *protogen.Method) string {
		return fmt.Sprintf("%s", method.Parent.Desc.Name())
	},
	"streamMethods":         streamMethods,
	"qspecServices":         qspecServices,
	"qspecMethods":          qspecMethods,
	"unexport":              unexport,
	"mapInternalOutType":    mapInternalOutType,
	"mapCorrectableOutType": mapCorrectableOutType,
	"mapFutureOutType":      mapFutureOutType,
	"qcresult":              qcresult,
	"contains":              strings.Contains,
}

func out(g *protogen.GeneratedFile, method *protogen.Method) string {
	return g.QualifiedGoIdent(method.Output.GoIdent)
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

func internalOut(g *protogen.GeneratedFile, method *protogen.Method) string {
	out := g.QualifiedGoIdent(method.Output.GoIdent)
	return fmt.Sprintf("internal%s", out[strings.LastIndex(out, ".")+1:])
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
