package gengorums

var serverVariables = `
{{- $context := use "gorums.ServerCtx" .GenFile}}
{{- $gorumsMessage := use "gorums.Message" .GenFile}}
{{- $wrapMessage := use "gorums.WrapMessage" .GenFile}}
{{- $sendMessage := use "gorums.SendMessage" .GenFile}}
`

var serverServicesBegin = `
{{- $genFile := .GenFile}}
{{- range configurationsServices .Services}}
	{{- $service := .GoName}}
	{{- $serverName := serviceTypeName $service "Server"}}
`

var serverServicesEnd = `
{{- end}}
`

var serverInterface = `
// {{$serverName}} is the server-side API for the {{$service}} Service
type {{$serverName}} interface {
	{{- range .Methods}}
		{{- if isOneway .}}
			{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}})
		{{- else if isStream .}}
			{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, send func(response *{{out $genFile .}}) error) error
		{{- else}}
			{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}) (response *{{out $genFile .}}, err error)
		{{- end}}
	{{- end}}
}
`

var registerInterface = `
func Register{{$serverName}}(srv *{{use "gorums.Server" $genFile}}, impl {{$serverName}}) {
	{{- range .Methods}}
	srv.RegisterHandler("{{.Desc.FullName}}", func(ctx {{$context}}, in *{{$gorumsMessage}}, {{if isOneway .}} _ {{- else}} finished {{- end}} chan<- *{{$gorumsMessage}}) {
		req := in.Message.(*{{in $genFile .}})
		defer ctx.Release()
		{{- if isOneway .}}
			impl.{{.GoName}}(ctx, req)
		{{- else if isStream .}}
			err := impl.{{.GoName}}(ctx, req, func(resp *{{out $genFile .}}) error {
				// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
				md := {{use "proto.Clone" $genFile}}(in.Metadata)
				return {{$sendMessage}}(ctx, finished, {{$wrapMessage}}(md.(*{{use "ordering.Metadata" $genFile}}), resp, nil))
			})
			if err != nil {
				{{$sendMessage}}(ctx, finished, {{$wrapMessage}}(in.Metadata, nil, err))
			}
		{{- else }}
			resp, err := impl.{{.GoName}}(ctx, req)
			{{$sendMessage}}(ctx, finished, {{$wrapMessage}}(in.Metadata, resp, err))
		{{- end}}
	})
	{{- end}}
}
`

var server = serverVariables +
	serverServicesBegin +
	serverInterface + registerInterface +
	serverServicesEnd
