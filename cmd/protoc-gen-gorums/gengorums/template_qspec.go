package gengorums

import (
	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

var qspecInterface = `
{{$cmpOrdered := use "cmp.Ordered" .GenFile}}
{{$genFile := .GenFile}}
{{$configOpt := use "gorums.ConfigOption" .GenFile}}
{{- range qspecServices .Services}}
// QuorumSpec is the interface of quorum functions for {{.GoName}}.
type QuorumSpec[idType {{$cmpOrdered}}] interface {
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
	{{$method}}QF(in *{{$in}}, replies map[idType]*{{$out}}) (*{{$customOut}}{{withCorrectable . ", int"}}, bool)
{{end}}
}
{{end}}
`

// qspecMethods returns all Gorums methods that require
// a quorum function; that is, all except multicast and plain gRPC methods.
func qspecMethods(methods []*protogen.Method) (s []*protogen.Method) {
	for _, method := range methods {
		if hasMethodOption(method, gorums.E_Multicast, gorums.E_Unicast) || !hasGorumsCallType(method) {
			// ignore multicast and non-Gorums methods
			continue
		}
		s = append(s, method)
	}
	return s
}

// qspecServices returns all services that have Gorums methods.
func qspecServices(services []*protogen.Service) (s []*protogen.Service) {
	for _, service := range services {
		for _, method := range service.Methods {
			if !hasGorumsCallType(method) {
				// ignore services without Gorums methods
				continue
			}
		}
		s = append(s, service)
	}
	return s
}
