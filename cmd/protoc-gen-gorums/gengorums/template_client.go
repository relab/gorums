package gengorums

import (
	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

// isQuorumCall returns true if the method is a plain quorum call (without async).
func isQuorumCall(method *protogen.Method) bool {
	return hasMethodOption(method, gorums.E_Quorumcall) && !hasMethodOption(method, gorums.E_Async)
}

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
	// Note: Quorum call methods are standalone functions and not part of this interface.
	type {{$interfaceName}} interface {
		{{- range configurationInterfaceMethods .Methods}}
			{{- $method := .GoName}}
			{{- if isOneway .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}}, opts ...{{$callOpt}})
			{{- else if or (isCorrectable .) (isAsync .)}}
				{{- $out := out $genFile .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}}) *{{outType . $out}}
			{{- else}}
				{{- $out := out $genFile .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}}, opts ...{{$callOpt}}) (resp *{{$out}}, err error)
			{{- end}}
		{{- end}}
	}
	{{- if hasConfigurationInterfaceMethods .Methods}}
	// enforce interface compliance
	var _ {{$interfaceName}} = (*Configuration)(nil)
	{{- end}}
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
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}}, opts ...{{$callOpt}})
			{{- else}}
				{{- $out := out $genFile .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}}) (resp *{{$out}}, err error)
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

// configurationInterfaceMethods returns multi node methods that are still methods on Configuration.
// This excludes quorumcall methods which are now standalone functions.
func configurationInterfaceMethods(methods []*protogen.Method) (s []*protogen.Method) {
	for _, method := range methods {
		if hasConfigurationCallType(method) && !isQuorumCall(method) {
			s = append(s, method)
		}
	}
	return s
}

// hasConfigurationInterfaceMethods returns true if there are any methods that should be in the Configuration interface.
func hasConfigurationInterfaceMethods(methods []*protogen.Method) bool {
	return len(configurationInterfaceMethods(methods)) > 0
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
		if !hasConfigurationCallType(method) {
			s = append(s, method)
		}
	}
	return s
}
