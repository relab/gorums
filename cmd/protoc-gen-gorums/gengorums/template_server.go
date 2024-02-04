package gengorums

var serverVariables = `
{{$context := use "gorums.ServerCtx" .GenFile}}
{{$broadcastContext := use "gorums.BroadcastCtx" .GenFile}}
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
	{{.GoName}}(ctx {{$broadcastContext}}, request *{{in $genFile .}}, broadcast *Broadcast) (err error)
	{{- else}}
	{{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}) (response *{{out $genFile .}}, err error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

// {{.GoName}}(ctx {{$context}}, request *{{in $genFile .}}, broadcast *Broadcast) (response *{{out $genFile .}}, err error)

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

var registerInterfaceTest = `
{{$genFile := .GenFile}}
func (b *Broadcast) ReturnToClient(resp *ClientResponse, err error) {
	b.SetReturnToClient(resp, err)
}

func (srv *Server) ReturnToClient(resp *ClientResponse, err error, broadcastID string) {
	go srv.RetToClient(resp, err, broadcastID)
}
`

/*
func (srv *Server) RegisterConfiguration(c *Configuration) {
	srv.RegisterBroadcastFunc("dev.ZourmsService.QuorumCall", gorums.RegisterBroadcastFunc(c.QuorumCall))
	srv.ListenForBroadcast()
}

func (b *Broadcast) ReturnToClient(resp *Response, err error) {
	b.SetReturnToClient(resp, err)
}

func (b *Broadcast) PrePrepare(req *Request) {
	b.SetBroadcastValues("protos.PBFTNode.PrePrepare", req)
}
*/

var server = serverVariables + serverInterface + registerInterface + registerInterfaceTest
