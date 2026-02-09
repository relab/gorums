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
	{{- else if isStreamingServer .}}
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
{{$asProto := use "gorums.AsProto" .GenFile}}
{{$newMessage := use "gorums.NewResponseMessage" $genFile}}
{{range .Services -}}
{{$service := .GoName}}
func Register{{$service}}Server(srv *{{use "gorums.Server" $genFile}}, impl {{$service}}Server) {
	{{- range .Methods}}
	srv.RegisterHandler("{{.Desc.FullName}}", func(ctx {{$context}}, in *{{$gorumsMessage}}) (*{{$gorumsMessage}}, error) {
		req := {{$asProto}}[*{{in $genFile .}}](in)
		{{- if isOneway .}}
		impl.{{.GoName}}(ctx, req)
		return nil, nil
		{{- else if isStreamingServer .}}
		err := impl.{{.GoName}}(ctx, req, func(resp *{{out $genFile .}}) error {
			out := {{$newMessage}}(in, resp)
			return ctx.SendMessage(out)
		})
		return nil, err
		{{- else }}
		resp, err := impl.{{.GoName}}(ctx, req)
		if err != nil {
			return nil, err
		}
		return {{$newMessage}}(in, resp), nil
		{{- end}}
	})
	{{- end}}
}
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
