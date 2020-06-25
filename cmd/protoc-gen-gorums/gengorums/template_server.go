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
	{{- else}}
	{{.GoName}}(*{{in $genFile .}}, chan<- *{{out $genFile .}})
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
		{{- if isOneway .}}
		srv.{{.GoName}}(req)
		{{- else }}
		c := make(chan *{{out $genFile .}})
		go func() {
			resp := <-c
			finished <- &gorumsMessage{metadata: in.metadata, message: resp}
		}()
		srv.{{.GoName}}(req, c)
		{{- end}}
	}
	{{- end}}
}
{{- end}}
`

var server = serverInterface + registerInterface
