package gengorums

var serverVariables = `
{{$context := use "gorums.ServerCtx" .GenFile}}
{{$codes := use "codes.Code" .GenFile}}
{{$status := use "status.Status" .GenFile}}
`

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the server-side API for the {{$service}} Service
type {{$service}} interface {
	{{- range .Methods}}
	{{- if isBroadcast  .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, broadcast *Broadcast)
	{{- else if correctableStream .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, send func(response *{{out $genFile .}}) error) error
	{{- else if isOneway .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}})
	{{- else}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}) (response *{{out $genFile .}}, err error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

var registerServerMethods = `
{{$genFile := .GenFile}}

{{range .Services -}}
{{$service := .GoName}}
{{- range .Methods}}
{{- if isOneway .}}
func (srv *Server) {{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}) {
	panic(status.Errorf(codes.Unimplemented, "method {{.GoName}} not implemented"))
}
{{- else if correctableStream .}}
func (srv *Server) {{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, send func(response *{{out $genFile .}}) error) error {
	panic(status.Errorf(codes.Unimplemented, "method {{.GoName}} not implemented"))
}
{{- else if isBroadcast .}}
func (srv *Server) {{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method {{.GoName}} not implemented"))
}
{{- else}}
func (srv *Server) {{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}) (response *{{out $genFile .}}, err error) {
	panic(status.Errorf(codes.Unimplemented, "method {{.GoName}} not implemented"))
}
{{- end}}
{{- end}}
{{- end}}
`

var registerInterface = `
{{$genFile := .GenFile}}
{{$gorumsMessage := use "gorums.Message" .GenFile}}
{{$wrapMessage := use "gorums.WrapMessage" $genFile}}
{{$sendMessage := use "gorums.SendMessage" $genFile}}
{{range .Services -}}
{{$service := .GoName}}
func Register{{$service}}Server(srv *Server, impl {{$service}}) {
	{{- range .Methods}}
	{{- if isBroadcast .}}
	srv.RegisterHandler("{{.Desc.FullName}}", gorums.BroadcastHandler(impl.{{.GoName}}, srv.Server))
	{{- if isBroadcastCall .}}
	srv.RegisterClientHandler("{{.Desc.FullName}}")
	{{- end}}
	{{- else }}
	srv.RegisterHandler("{{.Desc.FullName}}", func(ctx {{$context}}, in *{{$gorumsMessage}}, {{if isOneway .}} _ {{- else}} finished {{- end}} chan<- *{{$gorumsMessage}}) {
		req := in.Message.(*{{in $genFile .}})
		defer ctx.Release()
		{{- if isOneway .}}
		impl.{{.GoName}}(ctx, req)
		{{- else if correctableStream .}}
		err := impl.{{.GoName}}(ctx, req, func(resp *{{out $genFile .}}) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := {{use "proto.Clone" $genFile}}(in.Metadata)
			return {{$sendMessage}}(ctx, finished, {{$wrapMessage}}(md.(*{{use "ordering.Metadata" $genFile}}), resp, nil))
		})
		if err != nil {
			{{$sendMessage}}(ctx, finished, {{$wrapMessage}}(in.Metadata, nil, err))
		}
		{{- else }}
		resp, err := impl.{{.GoName}}(ctx, req)
		{{$sendMessage}}(ctx, finished, {{$wrapMessage}}(in.Metadata, resp, err))
		{{- end}}
	})
	{{- end}}
	{{- end}}
	srv.RegisterHandler(gorums.Cancellation, gorums.BroadcastHandler(gorums.CancelFunc, srv.Server))
}
{{- end}}
`

var registerServerBroadcast = `
{{$genFile := .GenFile}}

{{range .Services -}}
{{$service := .GoName}}
{{- range .Methods}}
{{- if isBroadcastOption .}}

func (srv *Server) Broadcast{{.GoName}}(req *{{in $genFile .}}, opts... gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	if options.RelatedToReq > 0 {
		srv.broadcast.orchestrator.BroadcastHandler("{{.Desc.FullName}}", req, options.RelatedToReq, options)
	} else {
		srv.broadcast.orchestrator.ServerBroadcastHandler("{{.Desc.FullName}}", req, options)
	}
}

{{- end}}
{{- end}}
{{- end}}
`

var registerMethodConstants = `
{{$genFile := .GenFile}}
{{$gorumsMessage := use "gorums.Message" .GenFile}}
const (
{{range .Services -}}
{{$service := .GoName}}
{{- range .Methods}}
{{- if isBroadcast .}}
	{{$service}}{{.GoName}} string = "{{.Desc.FullName}}"
{{- else }}
	{{- if isBroadcastCall .}}
	{{$service}}{{.GoName}} string = "{{.Desc.FullName}}"
	{{- end}}
{{- end}}
{{- end}}
{{- end}}
)
`

var server = serverVariables + serverInterface + registerServerMethods + registerInterface + registerServerBroadcast + registerMethodConstants
