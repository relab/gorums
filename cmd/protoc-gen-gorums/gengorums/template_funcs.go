package gengorums

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

// importMap maps a package name to its import path. The templates' use
// function resolves a "pkg.Ident" reference through it, and [addImport]
// adds the packages that [staticCode] imports.
var importMap = map[string]protogen.GoImportPath{
	"context":    protogen.GoImportPath("context"),
	"gorums":     protogen.GoImportPath("github.com/relab/gorums"),
	"gorumsimpl": protogen.GoImportPath("github.com/relab/gorums/runtime/gorumsimpl"),
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
	"isStreamingServer": func(method *protogen.Method) bool {
		return method.Desc.IsStreamingServer()
	},
	"fullName": func(method *protogen.Method) string {
		return fmt.Sprintf("/%s/%s", method.Parent.Desc.FullName(), method.Desc.Name())
	},
	"in": func(g *protogen.GeneratedFile, method *protogen.Method) string {
		return g.QualifiedGoIdent(method.Input.GoIdent)
	},
	"isOneway": func(method *protogen.Method) bool {
		return hasMethodOption(method, gorums.E_Multicast, gorums.E_Unicast)
	},
	"out":                   out,
	"methodTypes":           methodTypes,
	"localNames":            localNames,
	"mapCorrectableOutType": mapCorrectableOutType,
	"unexport":              unexport,
}

// localNames maps each of the space-separated base names to a local name for
// generated code. A local name is the base name, with an underscore appended
// until it differs from each name that refs use at top level and from the
// other local names. Each ref is a string or a []string. A qualified ref,
// such as "node.Request", uses its package name, here node; an unqualified
// ref uses itself. Pass every identifier that the code in the scope of the
// local names refers to, so that no local name hides an imported package.
func localNames(bases string, refs ...any) map[string]string {
	taken := make(map[string]bool)
	use := func(ref string) {
		name, _, _ := strings.Cut(ref, ".")
		taken[name] = true
	}
	for _, ref := range refs {
		switch ref := ref.(type) {
		case string:
			use(ref)
		case []string:
			for _, r := range ref {
				use(r)
			}
		default:
			panic(fmt.Sprintf("localNames: unexpected ref type %T", ref))
		}
	}
	names := make(map[string]string)
	for base := range strings.FieldsSeq(bases) {
		name := base
		for taken[name] {
			name += "_"
		}
		taken[name] = true
		names[base] = name
	}
	return names
}

// methodTypes returns the qualified input and output types of the service's
// methods.
func methodTypes(g *protogen.GeneratedFile, service *protogen.Service) []string {
	var types []string
	for _, method := range service.Methods {
		types = append(types, g.QualifiedGoIdent(method.Input.GoIdent), out(g, method))
	}
	return types
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

func mapCorrectableOutType(g *protogen.GeneratedFile, services []*protogen.Service) (s map[string]string) {
	return mapType(g, services, func(g *protogen.GeneratedFile, method *protogen.Method, s map[string]string) {
		// Generate Correctable type aliases for quorumcall methods since
		// users can call .Correctable() on any quorum call result
		if hasMethodOption(method, gorums.E_Quorumcall) {
			o := out(g, method)
			corrOut := fmt.Sprintf("Correctable%s", field(o))
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
