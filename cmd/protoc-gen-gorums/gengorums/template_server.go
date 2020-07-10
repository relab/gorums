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
	{{- range nodeStreamMethods .Methods}}
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
{{range .Services -}}
{{$service := .GoName}}
func (s *GorumsServer) Register{{$service}}Server(srv {{$service}}) {
	{{- range nodeStreamMethods .Methods}}
	s.srv.handlers[{{unexport .GoName}}MethodID] = func(ctx {{$context}}, in *gorumsMessage ,{{if isOneway .}} _ {{- else}} finished {{- end}} chan<- *gorumsMessage) {
		req := in.message.(*{{in $genFile .}})
		{{- if isOneway .}}
		srv.{{.GoName}}(ctx, req)
		{{- else }}
		once := new({{use "sync.Once" $genFile}})
		f := func(resp *{{out $genFile .}}, err error) {
			{{- /* Only one response message is supported */ -}}
			once.Do(func() {
				finished <- wrapMessage(in.metadata, resp, err)
			})
		}
		srv.{{.GoName}}(ctx, req, f)
		{{- end}}
	}
	{{- end}}
}
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
