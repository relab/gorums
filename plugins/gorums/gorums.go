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
	*generator.Generator
	logger    *log.Logger
	templates []tmpl
	pkgData   struct {
		PackageName           string
		Clients               []string
		Services              []serviceMethod
		ResponseTypes         []responseType
		InternalResponseTypes []responseType
		IgnoreImports         bool
	}
}

// Name returns the name of this plugin, "gorums".
func (g *gorums) Name() string {
	return "gorums"
}

// commonDefSuffix is the name suffix for common definitions template files.
const commonDefSuffix = "common_definitions_tmpl"

// Init initializes the plugin.
func (g *gorums) Init(gen *generator.Generator) {
	g.Generator = gen

	g.logger = log.New(os.Stdout, "gorums: ", 0)
	if !logEnabled() {
		g.logger.SetOutput(ioutil.Discard)
	}

	g.templates = make([]tmpl, 0, len(templates))
	for name, devTemplate := range templates {
		if strings.HasSuffix(name, commonDefSuffix) {
			// Ignore common definitions, they are extracted below.
			continue
		}
		// Check if the template name has a common definitions template file.
		prefix := strings.SplitN(name, "_", 2)[0]
		commonDefName := prefix + "_" + commonDefSuffix
		common := templates[commonDefName]
		t := tmpl{name, template.Must(template.New(name).Parse(common + "\n" + devTemplate))}
		g.templates = append(g.templates, t)
	}

	// Sort to ensure deterministic output.
	sort.Slice(g.templates, func(i, j int) bool {
		return g.templates[i].name < g.templates[j].name
	})
}

// Given a type name defined in a .proto, return its object.
// Also record that we're using it, to guarantee the associated import.
func (g *gorums) objectNamed(name string) generator.Object {
	g.RecordTypeUse(name)
	return g.ObjectNamed(name)
}

// Given a type name defined in a .proto, return its fully qualified name as we
// will print it.
func (g *gorums) fqTypeName(str string) string {
	return g.TypeName(g.objectNamed(str))
}

// Given a type name defined in a .proto, return its name as we will print it.
// The package name is not part of this name.
func (g *gorums) typeName(str string) string {
	return generator.CamelCaseSlice(g.objectNamed(str).TypeName())
}

type tmpl struct {
	name string
	*template.Template
}

// Generate generates code for the services in the given file.
func (g *gorums) Generate(file *generator.FileDescriptor) {
	if len(file.FileDescriptorProto.Service) == 0 {
		return
	}

	g.generateServiceMethods(file)

	g.P("\n// Reference Gorums specific imports to suppress errors if they are not otherwise used.")
	g.P("var _ = codes.OK")
	g.P("var _ = bytes.MinRead")

	if err := g.processTemplates(); err != nil {
		die(err)
	}

	if err := g.generateDevFiles(); err != nil {
		die(err)
	}

	g.P("/* Static resources */")
	g.P(staticResources)
}

func (g *gorums) processTemplates() error {
	g.pkgData.IgnoreImports = true
	for _, tmpl := range g.templates {
		out := new(bytes.Buffer)
		err := tmpl.Execute(out, g.pkgData)
		if err != nil {
			return fmt.Errorf("error executing template for %q: %v", tmpl.name, err)
		}
		if gocode := strings.TrimSpace(out.String()); len(gocode) > 0 {
			g.P()
			g.P("/* Code generated by protoc-gen-gorums - source: ", tmpl.name, " */")
			g.P()
			g.P(gocode)
		}
	}
	return nil
}

func (g *gorums) generateDevFiles() error {
	if !produceDevFiles() {
		return nil
	}

	g.pkgData.IgnoreImports = false
	for _, tmpl := range g.templates {
		out := new(bytes.Buffer)
		out.WriteString("// Code generated by protoc-gen-gorums. DO NOT EDIT.\n")
		out.WriteString("// Source file to edit is: " + tmpl.name + "\n")

		if err := tmpl.Execute(out, g.pkgData); err != nil {
			return fmt.Errorf("error executing template for %q: %v", tmpl.name, err)
		}

		gocode, err := format.Source(out.Bytes())
		if err != nil {
			return fmt.Errorf("error formating code for %q: %v", tmpl.name, err)
		}

		genfile := filepath.Join(g.pkgData.PackageName, strings.Replace(tmpl.name, "_tmpl", "_gen.go", 1))
		if err = ioutil.WriteFile(genfile, gocode, 0644); err != nil {
			return fmt.Errorf("error writing to %q: %v", genfile, err)
		}
	}
	return nil
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
	g.P("\"google.golang.org/grpc/status\"")
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
	CustomReturnType     string

	FQRespName       string
	RespName         string
	FQCustomRespName string
	CustomRespName   string
	FQReqName        string
	CallType         string

	TypeName           string
	UnexportedTypeName string

	QuorumCall        bool
	Correctable       bool
	CorrectableStream bool
	Future            bool
	Multicast         bool
	QFWithReq         bool
	PerNodeArg        bool

	ClientStreaming bool
	ServerStreaming bool

	ServName        string // Redundant, but keeps it simple.
	ServPackageName string // Redundant, but makes it simpler.
}

type responseType struct {
	TypeName           string // Equal to UnexportedTypeName to facilitate reuse of this struct
	UnexportedTypeName string // Equal to TypeName to facilitate reuse of this struct
	FQRespName         string
	FQCustomRespName   string
	QuorumCall         bool
	Correctable        bool
	CorrectableStream  bool
	Future             bool
	Multicast          bool
}

func (g *gorums) generateServiceMethods(file *generator.FileDescriptor) {
	g.pkgData.PackageName = file.GetPackage()
	g.pkgData.Clients = make([]string, len(file.FileDescriptorProto.Service))
	smethods := make(map[string][]*serviceMethod)
	respTypes := make(map[string]*serviceMethod)
	internalRespTypes := make(map[string]*serviceMethod)
	for i, service := range file.FileDescriptorProto.Service {
		g.pkgData.Clients[i] = service.GetName() + "Client"
		for _, method := range service.Method {
			sm, err := verifyExtensionsAndCreate(service.GetName(), method)
			if err != nil {
				die(err)
			}
			if sm == nil {
				continue
			}
			// Package name ; keep a copy for each method, for convenience
			sm.ServPackageName = g.pkgData.PackageName
			// Service name ; keep a copy for each method, for convenience
			sm.ServName = service.GetName()
			// Request type with package (if needed)
			sm.FQReqName = g.fqTypeName(method.GetInputType())
			// Response type without package
			sm.RespName = g.typeName(method.GetOutputType())
			// Response type with package (if needed)
			sm.FQRespName = g.fqTypeName(method.GetOutputType())
			// Is it a client stream method
			sm.ClientStreaming = method.GetClientStreaming()
			// Is it a server stream method
			sm.ServerStreaming = method.GetServerStreaming()

			if sm.CustomReturnType == "" {
				sm.CustomRespName = sm.RespName
				sm.FQCustomRespName = sm.FQRespName
			} else {
				s := strings.Split(sm.CustomReturnType, ".")
				sm.CustomRespName = strings.Join(s[len(s)-1:], "")
				sm.FQCustomRespName = sm.CustomReturnType
			}

			sm.MethodName = generator.CamelCase(method.GetName())
			sm.UnexportedMethodName = unexport(sm.MethodName)

			sm.TypeName = sm.CallType + sm.CustomRespName
			sm.UnexportedTypeName = "internal" + sm.RespName
			respTypes[sm.TypeName] = sm
			internalRespTypes[sm.UnexportedTypeName] = sm

			methodsForName := smethods[sm.MethodName]
			methodsForName = append(methodsForName, sm)
			smethods[sm.MethodName] = methodsForName
		}
	}
	g.pkgData.Services = flattenDuplicateServiceMethods(smethods)
	g.pkgData.ResponseTypes = sortedResponseTypes(respTypes)
	g.pkgData.InternalResponseTypes = sortedResponseTypes(internalRespTypes)
}

func sortedResponseTypes(respTypes map[string]*serviceMethod) (responseTypes []responseType) {
	for respType, sm := range respTypes {
		responseTypes = append(responseTypes, responseType{
			TypeName:           respType,
			UnexportedTypeName: respType,
			FQRespName:         sm.FQRespName,
			FQCustomRespName:   sm.FQCustomRespName,
			QuorumCall:         sm.QuorumCall,
			Future:             sm.Future,
			Correctable:        sm.Correctable,
			CorrectableStream:  sm.CorrectableStream,
			Multicast:          sm.Multicast,
		})
	}
	sort.Slice(responseTypes, func(i, j int) bool {
		return responseTypes[i].TypeName < responseTypes[j].TypeName
	})
	return
}

// Check for and flatten duplicate method names across multiple services.
// Duplicates methods names are prefixed with the service name.
func flattenDuplicateServiceMethods(smethods map[string][]*serviceMethod) (allRewrittenFlat []serviceMethod) {
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
				allRewrittenFlat = append(allRewrittenFlat, *sm)
			}
		}
	}
	sort.Slice(allRewrittenFlat, func(i, j int) bool {
		if allRewrittenFlat[i].ServName < allRewrittenFlat[j].ServName {
			return true
		} else if allRewrittenFlat[i].ServName > allRewrittenFlat[j].ServName {
			return false
		} else {
			if allRewrittenFlat[i].MethodName < allRewrittenFlat[j].MethodName {
				return true
			} else if allRewrittenFlat[i].MethodName > allRewrittenFlat[j].MethodName {
				return false
			} else {
				return false
			}
		}
	})
	return
}

func verifyExtensionsAndCreate(service string, method *pb.MethodDescriptorProto) (*serviceMethod, error) {
	sm := &serviceMethod{
		QuorumCall:        hasQuorumCallExtension(method),
		Future:            hasFutureExtension(method),
		Correctable:       hasCorrectableExtension(method),
		CorrectableStream: hasCorrectableStreamExtension(method),
		Multicast:         hasMulticastExtension(method),
		QFWithReq:         hasQFWithReqExtension(method),
		PerNodeArg:        hasPerNodeArgExtension(method),
		CustomReturnType:  getCustomReturnTypeExtension(method),
	}

	mutuallyIncompatible := map[string]bool{
		"QuorumCall":        sm.QuorumCall,
		"Future":            sm.Future,
		"Correctable":       sm.Correctable,
		"CorrectableStream": sm.CorrectableStream,
		"Multicast":         sm.Multicast,
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
	sm.CallType = firstOption

	isQuorumCallVariant := isQuorumCallVariant(sm)

	switch {
	case !isQuorumCallVariant && sm.CustomReturnType != "":
		// Only QC variants can define custom return type
		// because we want to avoid rewriting the plain gRPC methods.
		return nil, fmt.Errorf(
			"%s.%s: cannot combine non-quorum call method with the '%s' option",
			service, method.GetName(), customReturnTypeOptionName(),
		)

	case !isQuorumCallVariant && sm.QFWithReq:
		// Only QC variants need to process replies.
		return nil, fmt.Errorf(
			"%s.%s: cannot combine non-quorum call method with the '%s' option",
			service, method.GetName(), qfRequestOptionName(),
		)

	case !sm.Multicast && method.GetClientStreaming():
		return nil, fmt.Errorf(
			"%s.%s: client-server streams is only valid with the '%s' option",
			service, method.GetName(), multicastOptionName(),
		)

	case sm.Multicast && !method.GetClientStreaming():
		return nil, fmt.Errorf(
			"%s.%s: '%s' option is only valid for client-server streams methods",
			service, method.GetName(), multicastOptionName(),
		)

	case !sm.CorrectableStream && method.GetServerStreaming():
		return nil, fmt.Errorf(
			"%s.%s: server-client streams is only valid with the '%s' option",
			service, method.GetName(), correctableStreamOptionName(),
		)

	case sm.CorrectableStream && !method.GetServerStreaming():
		return nil, fmt.Errorf(
			"%s.%s: '%s' option is only valid for server-client streams",
			service, method.GetName(), correctableStreamOptionName(),
		)

	case !isQuorumCallVariant && !sm.Multicast:
		// Plain gRPC; no further processing is done by gorums plugin.
		return nil, nil

	default:
		// All good.
		return sm, nil
	}
}

func isQuorumCallVariant(sm *serviceMethod) bool {
	return sm.QuorumCall || sm.Future || sm.Correctable || sm.CorrectableStream
}
