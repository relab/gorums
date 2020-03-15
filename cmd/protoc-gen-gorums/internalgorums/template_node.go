package internalgorums

import (
	"google.golang.org/protobuf/compiler/protogen"
)

var nodeServices = `
type nodeServices struct {
	{{range .Services}}
	{{.GoName}}Client
	{{- $serviceName := .GoName}}
	{{- range streamMethods .Methods}}
	{{$serviceName}}_{{.GoName}}Client
	{{- end -}}
	{{- end}}
}
`

var nodeConnectStream = `
{{$context := use "context.Context" .GenFile}}
{{$errorf := use "fmt.Errorf" .GenFile}}
func (n *Node) connectStream(ctx {{$context}}) (err error) {
	{{- range .Services}}
	n.{{.GoName}}Client = New{{.GoName}}Client(n.conn)
	{{- end}}

	{{- range .Services -}}
	{{$serviceName := .GoName}}
	{{- range streamMethods .Methods}}
	n.{{$serviceName}}_{{.GoName}}Client, err = n.{{$serviceName}}Client.{{.GoName}}(ctx)
  	if err != nil {
  		return {{$errorf}}("stream creation failed: %v", err)
  	}
	{{- end -}}
	{{end}}
	return nil
}
`

var nodeCloseStream = `
func (n *Node) closeStream() (err error) {
	{{- range .Services -}}
	{{- range streamMethods .Methods}}
	_, err = n.CloseAndRecv()
	{{- end -}}
	{{end}}
	return err
}
`

var node = nodeServices + nodeConnectStream + nodeCloseStream

// streamMethods returns all methods that support client streaming.
func streamMethods(methods []*protogen.Method) []*protogen.Method {
	var s []*protogen.Method
	for _, method := range methods {
		if method.Desc.IsStreamingClient() {
			s = append(s, method)
		}
	}
	return s
}
