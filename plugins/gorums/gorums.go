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

// Given a type name defined in a .proto, return its name as we will print it.
func (g *gorums) typeName(str string) string {
	return g.gen.TypeName(g.objectNamed(str))
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

var ignoreImport = map[string]bool{
	"fmt":                      true,
	"math":                     true,
	"strings":                  true,
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
	OrigName             string
	MethodName           string
	UnexportedMethodName string
	RPCName              string

	RespName string
	ReqName  string

	TypeName           string
	UnexportedTypeName string

	QuorumCall  bool
	Correctable bool
	Future      bool
	Multicast   bool

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
		if p[i].OrigName < p[j].OrigName {
			return true
		} else if p[i].OrigName > p[j].OrigName {
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

			sm.OrigName = method.GetName()
			sm.MethodName = generator.CamelCase(sm.OrigName)
			sm.RPCName = sm.MethodName // sm.MethodName may be overwritten if method name conflict
			sm.UnexportedMethodName = unexport(sm.MethodName)
			sm.RespName = g.typeName(method.GetOutputType())
			sm.ReqName = g.typeName(method.GetInputType())
			sm.TypeName = sm.MethodName + "Reply"
			sm.UnexportedTypeName = unexport(sm.TypeName)
			sm.ServName = service.GetName()
			if sm.TypeName == sm.RespName {
				sm.TypeName += "_"
			}

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
				sm.OrigName = sm.ServName + sm.OrigName
				sm.MethodName = sm.ServName + sm.MethodName
				sm.UnexportedMethodName = unexport(sm.MethodName)
				sm.TypeName = sm.MethodName + "Reply"
				sm.UnexportedTypeName = unexport(sm.TypeName)
				if sm.TypeName == sm.RespName {
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
		QuorumCall:  hasQuorumCallExtension(method),
		Future:      hasFutureExtension(method),
		Correctable: hasCorrectableExtension(method),
		Multicast:   hasMulticastExtension(method),
	}

	switch {
	case sm.Future && !sm.QuorumCall:
		return nil, fmt.Errorf(
			"%s.%s: illegal combination combination of options: 'future' but not 'qrpc'",
			service, method.GetName(),
		)
	case !sm.QuorumCall && !sm.Correctable && !sm.Multicast:
		return nil, nil
	case (sm.QuorumCall || sm.Correctable) && sm.Multicast:
		return nil, fmt.Errorf(
			"%s.%s: illegal combination combination of options: both 'qrpc/correctable' and 'broadcast'",
			service, method.GetName(),
		)
	case sm.Multicast && !method.GetClientStreaming():
		return nil, fmt.Errorf(
			"%s.%s: 'broadcast' option only vaild for client-server streams methods",
			service, method.GetName(),
		)
	case method.GetServerStreaming():
		return nil, fmt.Errorf(
			"%s.%s: server-client streams are not supported by gorums",
			service, method.GetName(),
		)
	default:
		return sm, nil
	}
}
