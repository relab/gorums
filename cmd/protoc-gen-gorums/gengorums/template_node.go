package gengorums

import (
	"google.golang.org/protobuf/compiler/protogen"
)

var nodeServices = `
type nodeServices struct {
	{{range .Services}}
	{{if not (exclusivelyStrictOrdering .)}}
	{{.GoName}}Client
	{{- $serviceName := .GoName}}
	{{- range streamMethods .Methods}}
	{{unexport .GoName}}Client {{$serviceName}}_{{.GoName}}Client
	{{- end -}}
	{{- end -}}
	{{- end}}
}
`

var nodeConnectStream = `
{{$errorf := use "fmt.Errorf" .GenFile}}
func (n *Node) connectStream(ctx {{use "context.Context" .GenFile}}) (err error) {
	{{- range .Services}}
	{{if not (exclusivelyStrictOrdering .)}}
	n.{{.GoName}}Client = New{{.GoName}}Client(n.conn)
	{{- end -}}
	{{- end}}

	{{- range .Services -}}
	{{if not (exclusivelyStrictOrdering .)}}
	{{$serviceName := .GoName}}
	{{- range streamMethods .Methods}}
	n.{{unexport .GoName}}Client, err = n.{{$serviceName}}Client.{{.GoName}}(ctx)
	if err != nil {
		return {{$errorf}}("stream creation failed: %v", err)
	}
	{{- end -}}
	{{- end -}}
	{{end}}
	return nil
}
`

var nodeCloseStream = `
func (n *Node) closeStream() (err error) {
	{{- range .Services -}}
	{{if not (exclusivelyStrictOrdering .)}}
	{{- range streamMethods .Methods}}
	{{- if not .Desc.IsStreamingServer}}
	_, err = n.{{unexport .GoName}}Client.CloseAndRecv()
	{{- end -}}
	{{- end -}}
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
