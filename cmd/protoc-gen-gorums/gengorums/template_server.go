package gengorums

var serverVariables = `
{{$context := use "gorums.ServerCtx" .GenFile}}
`

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the server-side API for the {{$service}} Service
type {{$service}} interface {
	{{- range .Methods}}
	{{- if isOneway .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}})
	{{- else if correctableStream .}}
	{{.GoName}}(request *{{in $genFile .}}, send func(response *{{out $genFile .}}) error) error
	{{- else}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}) (response *{{out $genFile .}}, err error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

var registerInterface = `
{{$genFile := .GenFile}}
{{$gorumsMessage := use "gorums.Message" .GenFile}}
{{$wrapMessage := use "gorums.WrapMessage" $genFile}}
{{$sendMessage := use "gorums.SendMessage" $genFile}}
{{range .Services -}}
{{$service := .GoName}}
func Register{{$service}}Server(srv *{{use "gorums.Server" $genFile}}, impl {{$service}}) {
	{{- range .Methods}}
	srv.RegisterHandler("{{.Desc.FullName}}", func(ctx {{$context}}, in *{{$gorumsMessage}}, {{if isOneway .}} _ {{- else}} finished {{- end}} chan<- *{{$gorumsMessage}}) {
		req := in.Message.(*{{in $genFile .}})
		defer ctx.Release()
		{{- if isOneway .}}
		impl.{{.GoName}}(ctx, req)
		{{- else if correctableStream .}}
		err := impl.{{.GoName}}(req, func(resp *{{out $genFile .}}) error {
			return {{$sendMessage}}(ctx, finished, {{$wrapMessage}}(in.Metadata, resp, nil))
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
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
