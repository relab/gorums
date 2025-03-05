package gengorums

import (
	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

var qspecInterface = `
{{$genFile := .GenFile}}
{{$configOpt := use "gorums.ConfigOption" .GenFile}}
{{- $services := qspecServices .Services}}
{{- if eq (len $services) 0}}
	// There are no quorum calls.
	type QuorumSpec interface{}
{{- end}}
{{- range $services}}
	// QuorumSpec is the interface of quorum functions for {{.GoName}}.
	type QuorumSpec interface {
		{{$configOpt}}

		{{range qspecMethods .Methods -}}
			{{/* Below . is the method object */}}
			{{$method := .GoName}}
			{{$in := in $genFile .}}
			{{$out := out $genFile .}}
			{{$customOut := customOut $genFile .}}
			// {{$method}}QF is the quorum function for the {{$method}}
			// {{docName .}} call method. The in parameter is the request object
			// supplied to the {{$method}} method at call time, and may or may not
			// be used by the quorum function. If the in parameter is not needed
			// you should implement your quorum function with '_ *{{$in}}'.
			{{$method}}QF(in *{{$in}}, replies map[uint32]*{{$out}}) (*{{$customOut}}{{withCorrectable . ", int"}}, bool)
		{{end}}
	}
{{end}}
`

// qspecMethods returns all Gorums methods that require
// a quorum function; that is, all except multicast, unicast and plain gRPC methods.
func qspecMethods(methods []*protogen.Method) (s []*protogen.Method) {
	for _, method := range methods {
		if hasMethodOption(method, gorums.E_Multicast, gorums.E_Unicast) || !hasGorumsCallType(method) {
			// ignore multicast, unicast and non-Gorums methods
			continue
		}
		s = append(s, method)
	}
	return s
}

// qspecServices returns all services that have qspec methods.
func qspecServices(services []*protogen.Service) (s []*protogen.Service) {
	for _, service := range services {
		if len(qspecMethods(service.Methods)) > 0 {
			s = append(s, service)
		}
	}
	return s
}
