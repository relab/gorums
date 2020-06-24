package gengorums

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the server-side API for the {{$service}} Service
type {{$service}} interface {
	{{- range nodeStreamMethods .Methods}}
	{{- if isOneway .}}
	{{.GoName}}(*{{in $genFile .}})
	{{- else if hasAsyncHandler .}}
	{{.GoName}}(*{{in $genFile .}}, chan<- *{{out $genFile .}})
	{{- else }}
	{{.GoName}}(*{{in $genFile .}}) *{{out $genFile .}}
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
	s.srv.handlers[{{unexport .GoName}}MethodID] = func(in *gorumsMessage ,{{if isOneway .}} _ {{- else}} finished {{- end}} chan<- *gorumsMessage) {
		req := in.message.(*{{in $genFile .}})
		{{- if hasAsyncHandler .}}
		c := make(chan *{{out $genFile .}})
		srv.{{.GoName}}(req, c)
		go func() {
			resp := <-c
			finished <- &gorumsMessage{metadata: in.metadata, message: resp}
		}()
		{{- else }}
		{{- if isOneway .}}
		srv.{{.GoName}}(req)
		{{- else}}
		resp := srv.{{.GoName}}(req)
		finished <- &gorumsMessage{metadata: in.metadata, message: resp}
		{{- end}}
		{{- end}}
	}
	{{- end}}
}
{{- end}}
`

var server = serverInterface + registerInterface
