package gengorums

import "google.golang.org/protobuf/compiler/protogen"

// serverNames holds the package-level identifiers that [genServer] declares
// for a service. The server templates take them as data, so that the templates
// and [serviceIdents] agree on the names.
type serverNames struct {
	// Interface is the name of the server interface.
	Interface string
	// Register is the name of the function that registers an implementation.
	Register string
}

// serverNamesOf returns the identifiers that [genServer] declares for the
// given service.
func serverNamesOf(service *protogen.Service) serverNames {
	name := service.GoName + serverSuffix
	return serverNames{Interface: name, Register: registerPrefix + name}
}

// idents returns the names as a list.
func (n serverNames) idents() []string {
	return []string{n.Interface, n.Register}
}

// genServer generates the server interface and registration function of each
// service.
func genServer(g *protogen.GeneratedFile, services []*protogen.Service) {
	// serverService is a service with the names that the server templates
	// declare for it.
	type serverService struct {
		Service *protogen.Service
		Names   serverNames
	}
	// serverData is the data of the server template.
	type serverData struct {
		GenFile  *protogen.GeneratedFile
		Services []serverService
	}
	data := serverData{GenFile: g}
	for _, service := range services {
		data.Services = append(data.Services, serverService{service, serverNamesOf(service)})
	}
	g.P(mustExecute(parseTemplate("server", server), data))
}

var serverVariables = `
{{$context := use "gorums.ServerContext" .GenFile}}
`

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .Service.GoName}}
// {{.Names.Interface}} is the server-side API for the {{$service}} Service
type {{.Names.Interface}} interface {
	{{- range .Service.Methods}}
	{{- if isOneway .}}
	{{.GoName}}({{$context}}, *{{in $genFile .}})
	{{- else if isStreamingServer .}}
	{{.GoName}}({{$context}}, *{{in $genFile .}}, func(*{{out $genFile .}}))
	{{- else}}
	{{.GoName}}({{$context}}, *{{in $genFile .}}) (*{{out $genFile .}}, error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

var registerInterface = `
{{$genFile := .GenFile}}
{{$gorumsMessage := use "gorums.Message" .GenFile}}
{{$protoMessage := use "proto.Message" .GenFile}}
{{$asProto := use "gorums.AsProto" .GenFile}}
{{$newMessage := use "gorumsimpl.NewResponseMessage" $genFile}}
{{$gorumsServer := use "gorums.Server" $genFile}}
{{range .Services -}}
{{$names := .Names}}
{{$reg := localNames "srv impl" $context $gorumsMessage $asProto $newMessage (methodTypes $genFile .Service)}}

func {{$names.Register}}({{$reg.srv}} *{{$gorumsServer}}, {{$reg.impl}} {{$names.Interface}}) {
	{{- range .Service.Methods}}
	{{- $local := localNames "ctx in req resp err send" $context $gorumsMessage $protoMessage $asProto $newMessage (in $genFile .) (out $genFile .) $reg.impl}}
	{{$reg.srv}}.RegisterHandler("{{.Desc.FullName}}", func({{$local.ctx}} {{$context}}, {{$local.in}} *{{$gorumsMessage}}, {{if isStreamingServer .}}{{$local.send}}{{else}}_{{end}} func({{$protoMessage}})) (*{{$gorumsMessage}}, error) {
		{{$local.req}} := {{$asProto}}[*{{in $genFile .}}]({{$local.in}})
		{{- if isOneway .}}
		{{$reg.impl}}.{{.GoName}}({{$local.ctx}}, {{$local.req}})
		return nil, nil
		{{- else if isStreamingServer .}}
		{{$reg.impl}}.{{.GoName}}({{$local.ctx}}, {{$local.req}}, func({{$local.resp}} *{{out $genFile .}}) {
			{{$local.send}}({{$local.resp}})
		})
		return nil, nil
		{{- else }}
		{{$local.resp}}, {{$local.err}} := {{$reg.impl}}.{{.GoName}}({{$local.ctx}}, {{$local.req}})
		if {{$local.err}} != nil {
			return nil, {{$local.err}}
		}
		return {{$newMessage}}({{$local.in}}, {{$local.resp}}), nil
		{{- end}}
	})
	{{- end}}
}
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
