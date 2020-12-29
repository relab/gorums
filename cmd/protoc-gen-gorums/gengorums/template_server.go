package gengorums

var serverVariables = `
{{$context := use "context.Context" .GenFile}}
`

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the server-side API for the {{$service}} Service
type {{$service}} interface {
	{{- range .Methods}}
	{{- if isOneway .}}
	{{.GoName}}({{$context}}, *{{in $genFile .}})
	{{- else}}
	{{.GoName}}({{$context}}, *{{in $genFile .}}, func(*{{out $genFile .}}, error))
	{{- end}}
	{{- end}}
}
{{- end}}
`

var registerInterface = `
{{$genFile := .GenFile}}
{{$gorumsMessage := use "gorums.Message" .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
func Register{{$service}}Server(srv *{{use "gorums.Server" $genFile}}, impl {{$service}}) {
	{{- range .Methods}}
	srv.RegisterHandler("{{.Desc.FullName}}", func(ctx {{$context}}, in *{{$gorumsMessage}}, {{if isOneway .}} _ {{- else}} finished {{- end}} chan<- *{{$gorumsMessage}}) {
		req := in.Message.(*{{in $genFile .}})
		{{- if isOneway .}}
		impl.{{.GoName}}(ctx, req)
		{{- else }}
		once := new({{use "sync.Once" $genFile}})
		f := func(resp *{{out $genFile .}}, err error) {
			{{- /* Only one response message is supported */ -}}
			once.Do(func() {
				finished <- {{use "gorums.WrapMessage" $genFile}}(in.Metadata, resp, err)
			})
		}
		impl.{{.GoName}}(ctx, req, f)
		{{- end}}
	})
	{{- end}}
}
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
