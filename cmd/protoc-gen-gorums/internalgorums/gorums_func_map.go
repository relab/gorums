package internalgorums

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

// TODO(meling) consider making these import funcs generic: {{lookup .GenFile "io" "EOF"}}
var funcMap = template.FuncMap{
	//TODO(meling) avoid to use these imports; instead make separate funcs to return identifier string
	"quorumCallImports": func(g *protogen.GeneratedFile) string {
		g.QualifiedGoIdent(contextPackage.Ident("Context"))
		g.QualifiedGoIdent(timePackage.Ident("Duration"))
		g.QualifiedGoIdent(statusPackage.Ident("FromError"))
		g.QualifiedGoIdent(codesPackage.Ident("OK"))
		return ""
	},
	"eof": func(g *protogen.GeneratedFile) string {
		return g.QualifiedGoIdent(ioPackage.Ident("EOF"))
	},
	"mutex": func(g *protogen.GeneratedFile) string {
		return g.QualifiedGoIdent(syncPackage.Ident("Mutex"))
	},
	"context": func(g *protogen.GeneratedFile) string {
		return g.QualifiedGoIdent(contextPackage.Ident("Context"))
	},
	"errorf": func(g *protogen.GeneratedFile) string {
		return g.QualifiedGoIdent(fmtPackage.Ident("Errorf"))
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
	"opts": func(g *protogen.GeneratedFile, method *protogen.Method) string {
		return g.QualifiedGoIdent(grpcPackage.Ident("CallOption"))
	},
	"out": func(g *protogen.GeneratedFile, method *protogen.Method) string {
		return g.QualifiedGoIdent(method.Output.GoIdent)
	},
	"customOut": customOut,
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
	"callTypeName": func(method *protogen.Method) string {
		ext := protoimpl.X.MessageOf(method.Desc.Options()).Interface()
		if proto.HasExtension(ext, gorums.E_Qc) {
			return "quorum"
		}
		if proto.HasExtension(ext, gorums.E_QcFuture) {
			return "asynchronous quorum"
		}
		if proto.HasExtension(ext, gorums.E_Correctable) {
			return "correctable quorum"
		}
		if proto.HasExtension(ext, gorums.E_CorrectableStream) {
			return "correctable stream quorum"
		}
		// should not reach here
		panic(fmt.Sprintf("method %s does not support any quorum calls", method.GoName))
	},
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

func unexport(s string) string { return strings.ToLower(s[:1]) + s[1:] }
