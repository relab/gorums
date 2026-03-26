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
	{{.GoName}}({{$context}}, *{{in $genFile .}})
	{{- else if isStreamingServer .}}
	{{.GoName}}({{$context}}, *{{in $genFile .}}, func(*{{out $genFile .}}))
	{{- else}}
	{{.GoName}}({{$context}}, *{{in $genFile .}}) (*{{out $genFile .}}, error)
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
		impl.{{.GoName}}(ctx, req, func(resp *{{out $genFile .}}) {
			out := {{$newMessage}}(in, resp)
			ctx.SendMessage(out)
		})
		return nil, nil
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
