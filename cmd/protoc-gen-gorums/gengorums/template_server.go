package gengorums

var serverVariables = `
{{$context := use "gorums.ServerCtx" .GenFile}}
`

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the server-side API for the {{$service}} Service
type {{$service}}Server interface {
	{{- range .Methods}}
	{{- if isOneway .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}})
	{{- else if correctableStream .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, send func(response *{{out $genFile .}}) error) error
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
{{$newMessage := use "gorums.NewResponseMessage" $genFile}}
{{range .Services -}}
{{$service := .GoName}}
func Register{{$service}}Server(srv *{{use "gorums.Server" $genFile}}, impl {{$service}}Server) {
	{{- range .Methods}}
	srv.RegisterHandler("{{.Desc.FullName}}", func(ctx {{$context}}, in *{{$gorumsMessage}}) (*{{$gorumsMessage}}, error) {
		req := in.Message.(*{{in $genFile .}})
		{{- if isOneway .}}
		impl.{{.GoName}}(ctx, req)
		return nil, nil
		{{- else if correctableStream .}}
		err := impl.{{.GoName}}(ctx, req, func(resp *{{out $genFile .}}) error {
			// create a copy of the metadata, to avoid a data race between NewResponseMessage and SendMsg
			md := {{use "proto.CloneOf" $genFile}}(in.GetMetadata())
			return ctx.SendMessage({{$newMessage}}(md, resp))
		})
		return nil, err
		{{- else }}
		resp, err := impl.{{.GoName}}(ctx, req)
		return {{$newMessage}}(in.GetMetadata(), resp), err
		{{- end}}
	})
	{{- end}}
}
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
