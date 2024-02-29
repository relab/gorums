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
	{{- if isOneway .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}})
	{{- else if correctableStream .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, send func(response *{{out $genFile .}}) error) error
	{{- else if isBroadcast .}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, broadcast *Broadcast)
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
	srv.RegisterClientHandler("{{.Desc.FullName}}", gorums.ServerClientRPC("{{.Desc.FullName}}"))
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
}
{{- end}}
`

var registerReplyToClientHandlers = `
{{$genFile := .GenFile}}
{{$protoMessage := use "protoreflect.ProtoMessage" .GenFile}}
func (b *Broadcast) SendToClient(resp protoreflect.ProtoMessage, err error) {
	b.sp.ReturnToClientHandler(resp, err, b.metadata)
}

func (srv *Server) SendToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.RetToClient(resp, err, broadcastID)
}
`

var server = serverVariables + serverInterface + registerServerMethods + registerInterface + registerReplyToClientHandlers
