package gengorums

import (
	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

// gorums need to be imported in the zorums file
var clientVariables = `
{{$context := use "context.Context" .GenFile}}
{{$_ := use "gorums.EnforceVersion" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var clientConfigurationInterface = `
{{- $genFile := .GenFile}}
{{- range configurationsServices .Services}}
	{{- $service := .GoName}}
	{{- $interfaceName := printf "%sClient" $service}}
	// {{$interfaceName}} is the client interface for the {{$service}} service.
	type {{$interfaceName}} interface {
		{{- range configurationMethods .Methods}}
			{{- $method := .GoName}}
			{{- if isOneway .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}, opts ...{{$callOpt}})
			{{- else if or (isCorrectable .) (isAsync .)}}
				{{- $customOut := outType . (customOut $genFile .)}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) *{{$customOut}}
			{{- else if isBroadcastCall .}}
				{{- $customOut := customOut $genFile .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}, cancelOnTimeout ...bool) (resp *{{$customOut}}, err error)
			{{- else}}
				{{- $customOut := customOut $genFile .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) (resp *{{$customOut}}, err error)
			{{- end}}
		{{- end}}
	}
	// enforce interface compliance
	var _ {{$interfaceName}} = (*Configuration)(nil)
{{- end}}
`

var clientNodeInterface = `
{{- $genFile := .GenFile}}
{{- range nodeServices .Services}}
	{{- $service := .GoName}}
	{{- $interfaceName := printf "%sNodeClient" $service}}
	// {{$interfaceName}} is the single node client interface for the {{$service}} service.
	type {{$interfaceName}} interface {
		{{- range nodeMethods .Methods}}
			{{- $method := .GoName}}
			{{- if isOneway .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}, opts ...{{$callOpt}})
			{{- else}}
				{{- $customOut := customOut $genFile .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) (resp *{{$customOut}}, err error)
			{{- end}}
		{{- end}}
	}
	// enforce interface compliance
	var _ {{$interfaceName}} = (*Node)(nil)
{{- end}}
`

var client = clientVariables + clientConfigurationInterface + clientNodeInterface

// configurationsServices returns all services containing at least one multi node method.
func configurationsServices(services []*protogen.Service) (s []*protogen.Service) {
	for _, service := range services {
		if len(configurationMethods(service.Methods)) > 0 {
			s = append(s, service)
		}
	}
	return s
}

// configurationMethods returns all multi node methods, such as multicast, quorumcall, correctable, and async methods.
func configurationMethods(methods []*protogen.Method) (s []*protogen.Method) {
	for _, method := range methods {
		if hasConfigurationCallType(method) {
			s = append(s, method)
		}
	}
	return s
}

// nodeServices returns all services containing at least one single node method.
func nodeServices(services []*protogen.Service) (s []*protogen.Service) {
	for _, service := range services {
		if len(nodeMethods(service.Methods)) > 0 {
			s = append(s, service)
		}
	}
	return s
}

// nodeMethods returns all single node methods, such as unicast and plain gRPC methods.
func nodeMethods(methods []*protogen.Method) (s []*protogen.Method) {
	for _, method := range methods {
		// ignore internal broadcast methods
		if !hasConfigurationCallType(method) && !hasMethodOption(method, gorums.E_Broadcast) {
			s = append(s, method)
		}
	}
	return s
}
