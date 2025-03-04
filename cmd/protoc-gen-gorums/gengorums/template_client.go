package gengorums

import (
	"google.golang.org/protobuf/compiler/protogen"
)

// gorums need to be imported in the zorums file
var clientVariables = `
{{$context := use "context.Context" .GenFile}}
{{$_ := use "gorums.EnforceVersion" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var clientConfigurationInterface = `
{{$genFile := .GenFile}}
{{- range configurationsServices .Services}}
	{{$service := .GoName}}
	// {{$service}} is the client-side Configuration API for the {{$service}} Service
	type {{$service}}ConfigurationClient interface {
		{{- range configurationMethods .Methods}}
			{{- $method := .GoName}}
			{{- if isOneway .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}, opts ...{{$callOpt}})
			{{- else if or (isCorrectable .) (isAsync .)}}
				{{- $customOut := outType . (customOut $genFile .)}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) *{{$customOut}}
			{{- else}}
				{{- $customOut := customOut $genFile .}}
				{{$method}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) (resp *{{$customOut}}, err error)
			{{- end}}
		{{- end}}
	}
	// enforce interface compliance
	var _ {{$service}}ConfigurationClient = (*Configuration)(nil)
{{- end}}
`

var clientNodeInterface = `
{{$genFile := .GenFile}}
{{- range nodeServices .Services}}
{{$service := .GoName}}
// {{$service}} is the client-side Node API for the {{$service}} Service
type {{$service}}NodeClient interface {
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
var _ {{$service}}NodeClient = (*Node)(nil)
{{- end}}
`

var client = clientVariables + clientConfigurationInterface + clientNodeInterface

// gorumsMethods returns all Gorums-specific methods, such as multicast, quorumcall, correctable, and async methods.
func configurationMethods(methods []*protogen.Method) (s []*protogen.Method) {
	for _, method := range methods {
		if hasConfigurationCallType(method) {
			s = append(s, method)
		}
	}
	return s
}

// nodeOnlyServices returns all node-only services, such as services with only unicast and plain gRPC methods.
func nodeServices(services []*protogen.Service) (s []*protogen.Service) {
	for _, service := range services {
		for _, method := range service.Methods {
			if !hasConfigurationCallType(method) {
				s = append(s, service)
				break
			}
		}
	}
	return s
}

// nodeMethods returns all node-specific methods, such as unicast and plain gRPC methods.
func nodeMethods(methods []*protogen.Method) (s []*protogen.Method) {
	for _, method := range methods {
		if !hasConfigurationCallType(method) {
			s = append(s, method)
		}
	}
	return s
}

// gorumsServices returns all services that have Gorums methods.
func configurationsServices(services []*protogen.Service) (s []*protogen.Service) {
	for _, service := range services {
		for _, method := range service.Methods {
			if hasConfigurationCallType(method) {
				s = append(s, service)
				break
			}
		}
	}
	return s
}
