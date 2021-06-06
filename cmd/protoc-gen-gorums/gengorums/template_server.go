package gengorums

var serverVariables = `
{{$context := use "context.Context" .GenFile}}
{{$mutex := use "sync.Mutex" .GenFile}}
`

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the server-side API for the {{$service}} Service
type {{$service}} interface {
	{{- range .Methods}}
	{{- if isOneway .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, release func())
	{{- else}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, release func()) (response *{{out $genFile .}}, err error)
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
	srv.RegisterHandler("{{.Desc.FullName}}", func(ctx {{$context}}, in *{{$gorumsMessage}}, {{if isOneway .}} _ {{- else}} finished {{- end}} chan<- *{{$gorumsMessage}}, mut *{{$mutex}}) {
		req := in.Message.(*{{in $genFile .}})
		once := new({{use "sync.Once" $genFile}})
		release := func() { once.Do(mut.Unlock) }
		defer release()
		{{- if isOneway .}}
		impl.{{.GoName}}(ctx, req, release)
		{{- else }}
		resp, err := impl.{{.GoName}}(ctx, req, release)
		select {
		case finished <- {{use "gorums.WrapMessage" $genFile}}(in.Metadata, resp, err):
		case <-ctx.Done():
		}
		{{- end}}
	})
	{{- end}}
}
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
