// Go support for Protocol Buffers - Google's data interchange format
//
// Copyright 2015 The Go Authors.  All rights reserved.
// https://github.com/golang/protobuf
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//     * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//     * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// Package gorums outputs a gorums client API in Go code.
// It runs as a plugin for the Go protocol buffer compiler plugin.
// It is linked in to protoc-gen-gorums.
package gorums

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	pb "github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	"github.com/gogo/protobuf/protoc-gen-gogo/generator"
)

func init() {
	generator.RegisterPlugin(new(gorums))
}

// gorums is an implementation of the Go protocol buffer compiler's
// plugin architecture. It generates bindings for gorums support.
type gorums struct {
	gen       *generator.Generator
	logger    *log.Logger
	templates []tmpl
	pkgData   struct {
		PackageName   string
		Clients       []string
		Services      []serviceMethod
		IgnoreImports bool
	}
}

// Name returns the name of this plugin, "gorums".
func (g *gorums) Name() string {
	return "gorums"
}

// Init initializes the plugin.
func (g *gorums) Init(gen *generator.Generator) {
	g.gen = gen

	g.logger = log.New(os.Stdout, "gorums: ", 0)
	if !logEnabled() {
		g.logger.SetOutput(ioutil.Discard)
	}

	g.templates = make([]tmpl, 0, len(templates))
	for name, devTemplate := range templates {
		t := tmpl{
			name: name,
			t:    template.Must(template.New(name).Parse(devTemplate)),
		}
		g.templates = append(g.templates, t)
	}

	// sort to ensure deterministic output
	sort.Sort(tmplSlice(g.templates))
}

// Given a type name defined in a .proto, return its object.
// Also record that we're using it, to guarantee the associated import.
func (g *gorums) objectNamed(name string) generator.Object {
	g.gen.RecordTypeUse(name)
	return g.gen.ObjectNamed(name)
}

// Given a type name defined in a .proto, return its fully qualified name as we
// will print it.
func (g *gorums) fqTypeName(str string) string {
	return g.gen.TypeName(g.objectNamed(str))
}

// Given a type name defined in a .proto, return its name as we will print it.
// The package name is not part of this name.
func (g *gorums) typeName(str string) string {
	return generator.CamelCaseSlice(g.objectNamed(str).TypeName())
}

// P forwards to g.gen.P.
func (g *gorums) P(args ...interface{}) { g.gen.P(args...) }

type tmpl struct {
	name string
	t    *template.Template
}

type tmplSlice []tmpl

func (p tmplSlice) Len() int           { return len(p) }
func (p tmplSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p tmplSlice) Less(i, j int) bool { return p[i].name < p[j].name }

// Generate generates code for the services in the given file.
func (g *gorums) Generate(file *generator.FileDescriptor) {
	if len(file.FileDescriptorProto.Service) == 0 {
		return
	}

	g.pkgData.PackageName = file.GetPackage()
	g.pkgData.Clients, g.pkgData.Services = g.generateServiceMethods(file.FileDescriptorProto.Service)

	g.referenceToSuppressErrs()

	g.pkgData.IgnoreImports = true
	if err := g.processTemplates(); err != nil {
		die(err)
	}

	g.pkgData.IgnoreImports = false
	if err := g.generateDevFiles(); err != nil {
		die(err)
	}

	g.embedStaticResources()
}

func (g *gorums) referenceToSuppressErrs() {
	g.P()
	g.P("//  Reference Gorums specific imports to suppress errors if they are not otherwise used.")
	g.P("var _ = codes.OK")
}

func (g *gorums) processTemplates() error {
	for _, tmpl := range g.templates {
		out := new(bytes.Buffer)
		err := tmpl.t.Execute(out, g.pkgData)
		if err != nil {
			return fmt.Errorf("error executing template for %q: %v", tmpl.name, err)
		}
		g.P()
		g.P("/* 'gorums' plugin for protoc-gen-go - generated from: ", tmpl.name, " */")
		g.P()
		g.P(out.String())
	}

	return nil
}

func (g *gorums) generateDevFiles() error {
	if !produceDevFiles() {
		return nil
	}

	for _, tmpl := range g.templates {
		out := new(bytes.Buffer)
		out.WriteString("// DO NOT EDIT. Generated by 'gorums' plugin for protoc-gen-go\n")
		out.WriteString("// Source file to edit is: " + tmpl.name + "\n")

		err := tmpl.t.Execute(out, g.pkgData)
		if err != nil {
			return fmt.Errorf("error executing template for %q: %v", tmpl.name, err)
		}

		dst, err := format.Source(out.Bytes())
		if err != nil {
			return fmt.Errorf("error formating code for %q: %v", tmpl.name, err)
		}

		genfile := filepath.Join(g.pkgData.PackageName, strings.Replace(tmpl.name, "_tmpl", "_gen.go", 1))
		g.logger.Println("writing:", genfile)
		err = ioutil.WriteFile(genfile, dst, 0644)
		if err != nil {
			return fmt.Errorf("error writing file for %q: %v", tmpl.name, err)
		}
	}

	return nil
}

func (g *gorums) embedStaticResources() {
	g.P("/* Static resources */")
	g.P(staticResources)
}

// GenerateImports generates the import declaration for this file.
func (g *gorums) GenerateImports(file *generator.FileDescriptor) {
	if len(file.FileDescriptorProto.Service) == 0 {
		return
	}
	if len(staticImports) == 0 {
		return
	}

	if hasMessageWithByteField(file) {
		ignoreImport["bytes"] = true
	}

	if len(file.Messages()) > 0 {
		ignoreImport["io"] = true
		ignoreImport["strings"] = true
	}

	sort.Strings(staticImports)
	g.P("import (")
	for _, simport := range staticImports {
		if ignore := ignoreImport[simport]; ignore {
			continue
		}
		g.P("\"", simport, "\"")
	}
	g.P()
	g.P("\"golang.org/x/net/trace\"")
	g.P()
	g.P("\"google.golang.org/grpc/codes\"")
	g.P(")")
}

func hasMessageWithByteField(file *generator.FileDescriptor) bool {
	for _, msg := range file.Messages() {
		for _, field := range msg.Field {
			if field.IsBytes() {
				return true
			}
		}
	}
	return false
}

var ignoreImport = map[string]bool{
	"fmt":  true,
	"math": true,
	"golang.org/x/net/context": true,
	"golang.org/x/net/trace":   true,
	"google.golang.org/grpc":   true,
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "gorums: %v\n", err)
	os.Exit(2)
}

func produceDevFiles() bool {
	return os.Getenv("GORUMSGENDEV") == "1"
}

func logEnabled() bool {
	return os.Getenv("GORUMSLOG") == "1"
}

func unexport(s string) string { return strings.ToLower(s[:1]) + s[1:] }

type serviceMethod struct {
	MethodName           string
	UnexportedMethodName string
	RPCName              string
	MethodArg            string
	MethodArgUse         string
	MethodArgCall        string
	CustomReturnType     string

	FQRespName string
	// RespName   string //TODO remove if FQRespName is enough
	FQReqName string

	TypeName           string
	UnexportedTypeName string

	QuorumCall        bool
	Correctable       bool
	CorrectablePrelim bool
	Future            bool
	Multicast         bool
	QFWithReq         bool
	PerNodeArg        bool

	ServName string // Redundant, but keeps it simple.
}

type smSlice []serviceMethod

func (p smSlice) Len() int      { return len(p) }
func (p smSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p smSlice) Less(i, j int) bool {
	if p[i].ServName < p[j].ServName {
		return true
	} else if p[i].ServName > p[j].ServName {
		return false
	} else {
		if p[i].MethodName < p[j].MethodName {
			return true
		} else if p[i].MethodName > p[j].MethodName {
			return false
		} else {
			return false
		}
	}
}

func (g *gorums) generateServiceMethods(services []*pb.ServiceDescriptorProto) ([]string, []serviceMethod) {
	clients := make([]string, len(services))
	smethods := make(map[string][]*serviceMethod)
	for i, service := range services {
		clients[i] = service.GetName() + "Client"
		for _, method := range service.Method {
			sm, err := verifyExtensionsAndCreate(service.GetName(), method)
			if err != nil {
				die(err)
			}
			if sm == nil {
				continue
			}

			sm.MethodName = generator.CamelCase(method.GetName())
			sm.RPCName = sm.MethodName // sm.MethodName may be overwritten if method name conflict
			sm.UnexportedMethodName = unexport(sm.MethodName)

			sm.FQReqName = g.fqTypeName(method.GetInputType())
			sm.FQRespName = g.fqTypeName(method.GetOutputType())
			// sm.RespName = g.typeName(method.GetOutputType()) //TODO always same as FQRespName??
			sm.TypeName = sm.MethodName + "Reply"
			sm.UnexportedTypeName = unexport(sm.TypeName)
			sm.ServName = service.GetName()
			if sm.TypeName == sm.FQRespName {
				sm.TypeName += "_"
			}
			fmt.Fprintf(os.Stderr, "%v\n \tRPCName\t\t%v\n \tUnexpMethodName\t%v\n \tFQRespName\t%v\n \tFQReqName\t%v\n \tTypeName\t%v\n \tUnexpTypeName\t%v\n \tServName\t%v\n ",
				sm.MethodName, sm.RPCName, sm.UnexportedMethodName, sm.FQRespName, sm.FQReqName, sm.TypeName, sm.UnexportedTypeName, sm.ServName,
			)

			sm.MethodArg = "args *" + sm.FQReqName
			sm.MethodArgCall = "args"
			sm.MethodArgUse = "args"
			if sm.PerNodeArg {
				sm.MethodArg = "perNodeArg func(nodeID uint32) *" + sm.FQReqName
				sm.MethodArgCall = "perNodeArg(n.id)"
				sm.MethodArgUse = "perNodeArg"
				fmt.Fprintf(os.Stderr, "per_node %v -- %v\n", sm.MethodName, sm.MethodArg)
			}
			fmt.Fprintf(os.Stderr, "xxxxx %v -- %v (custom return type: %v)\n", sm.MethodName, sm.MethodArg, sm.CustomReturnType)

			//TODO Why do we need to check for equal methods?? gRPC does not allow equally named methods.
			methodsForName, _ := smethods[sm.MethodName]
			methodsForName = append(methodsForName, sm)
			smethods[sm.MethodName] = methodsForName
		}
	}

	var allRewrittenFlat []serviceMethod

	for _, methodsForName := range smethods {
		switch len(methodsForName) {
		case 0:
			panic("generateServiceMethods: found method name with no data")
		case 1:
			allRewrittenFlat = append(allRewrittenFlat, *methodsForName[0])
			continue
		default:
			for _, sm := range methodsForName {
				sm.MethodName = sm.ServName + sm.MethodName
				sm.UnexportedMethodName = unexport(sm.MethodName)
				sm.TypeName = sm.MethodName + "Reply"
				sm.UnexportedTypeName = unexport(sm.TypeName)
				if sm.TypeName == sm.FQRespName {
					sm.TypeName += "_"
				}
				allRewrittenFlat = append(allRewrittenFlat, *sm)
			}
		}
	}

	sort.Sort(smSlice(allRewrittenFlat))

	return clients, allRewrittenFlat
}

func verifyExtensionsAndCreate(service string, method *pb.MethodDescriptorProto) (*serviceMethod, error) {
	sm := &serviceMethod{
		QuorumCall:        hasQuorumCallExtension(method),
		Future:            hasFutureExtension(method),
		Correctable:       hasCorrectableExtension(method),
		CorrectablePrelim: hasCorrectablePRExtension(method),
		Multicast:         hasMulticastExtension(method),
		QFWithReq:         hasQFWithReqExtension(method),
		PerNodeArg:        hasPerNodeArgExtension(method),
		CustomReturnType:  getCustomReturnTypeExtension(method),
	}

	mutuallyIncompatible := map[string]bool{
		qcName():     sm.QuorumCall,
		futureName(): sm.Future,
		corrName():   sm.Correctable,
		corrPrName(): sm.CorrectablePrelim,
		mcastName():  sm.Multicast,
	}
	firstOption := ""
	for optionName, optionSet := range mutuallyIncompatible {
		if optionSet {
			if firstOption != "" {
				return nil, fmt.Errorf(
					"%s.%s: cannot combine options: '%s' and '%s'",
					service, method.GetName(), firstOption, optionName,
				)
			}
			firstOption = optionName
		}
	}

	isQuorumCallVariant := isQuorumCallVariant(sm)

	switch {
	case !isQuorumCallVariant && sm.CustomReturnType != "":
		// only QC variants can define custom return type
		// because we want to avoid rewriting the plain gRPC methods
		return nil, fmt.Errorf(
			"%s.%s: cannot combine non-quorum call method with the '%s' option",
			service, method.GetName(), custRetName(),
		)

	case !isQuorumCallVariant && sm.QFWithReq:
		// only QC variants need to process replies
		return nil, fmt.Errorf(
			"%s.%s: cannot combine non-quorum call method with the '%s' option",
			service, method.GetName(), qfreqName(),
		)

	case sm.Multicast && !method.GetClientStreaming():
		return nil, fmt.Errorf(
			"%s.%s: '%s' option is only valid for client-server streams methods",
			service, method.GetName(), mcastName(),
		)

	case !sm.CorrectablePrelim && method.GetServerStreaming():
		return nil, fmt.Errorf(
			"%s.%s: '%s' option is only valid for server-client streams",
			service, method.GetName(), corrPrName(),
		)

	case !isQuorumCallVariant && !sm.Multicast:
		// plain gRPC; no further processing is done by gorums plugin
		return nil, nil

	default:
		// all good
		return sm, nil
	}
}

func isQuorumCallVariant(sm *serviceMethod) bool {
	return sm.QuorumCall || sm.Future || sm.Correctable || sm.CorrectablePrelim
}
