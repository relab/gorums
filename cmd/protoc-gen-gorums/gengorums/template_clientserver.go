package gengorums

var clientServerVariables = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$grpc := use "grpc.GRPC" .GenFile}}
`

var clientServerInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// clientServer is the client server API for the {{$service}} Service
type clientServer interface {
	{{- range .Methods}}
	{{- if isBroadcastCall .}}
	client{{.GoName}}(ctx {{$context}}, request *{{out $genFile .}}) (any, error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

var clientServerDesc = `
{{$genFile := .GenFile}}
var clientServer_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "protos.ClientServer",
	HandlerType: (*clientServer)(nil),
	Methods: []grpc.MethodDesc{
		{
		{{range .Services -}}
		{{- range .Methods}}
		{{- if isBroadcastCall .}}
		MethodName: "Client{{.GoName}}",
		Handler:    _client{{.GoName}},
		{{- end}}
		{{- end}}
		{{- end}}
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "",
}
`

var clientServerImpl = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// clientServer is the client server API for the {{$service}} Service
type clientServerImpl struct {
	grpcServer *grpc.Server
	respChan chan *clientResponse
	reqChan chan string

	{{- range .Methods}}
	{{- if isBroadcastCall .}}
	handler{{.GoName}} func(resps []*{{out $genFile .}}) error
	resps{{.GoName}} map[string][]*{{out $genFile .}}
	doneChan{{.GoName}} chan *{{out $genFile .}}
	{{- end}}
	{{- end}}
}
{{- end}}
`

var clientServerImplInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// clientServer is the client server API for the {{$service}} Service
type ReplySpec interface {
	{{- range .Methods}}
	{{- if isBroadcastCall .}}
	{{.GoName}}(reqs []*{{out $genFile .}}) (*{{out $genFile .}}, error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

var clientServer = clientServerVariables + clientServerInterface + clientServerDesc + clientServerImpl + clientServerImplInterface

/*
func (c *Configuration) RegisterClientServer(listenAddr string, handler func(resps []*ClientResponse) (*ClientResponse, error)) {
	var opts []grpc.ServerOption
	srv := clientServerImpl{
		grpcServer: grpc.NewServer(opts...),
		respChan: make(chan *clientResponse, 10),
		reqChan: make(chan string),
		doneChan: make(chan *ClientResponse),
		resps: make(map[string][]*ClientResponse),
		handler: handler,
	}
	lis, err := net.Listen("tcp", listenAddr)
	for err != nil {
		return
	}
	srv.grpcServer.RegisterService(&clientServer_ServiceDesc, srv)
	go srv.grpcServer.Serve(lis)
	go srv.handle()
	c.srv = &srv
}

func (srv *clientServerImpl) handle() {
	for {
		select {
		case resp := <- srv.respChan:
			if _, ok := srv.resps[resp.broadcastID]; !ok {
				continue
			}
			srv.resps[resp.broadcastID] = append(srv.resps[resp.broadcastID], resp.data.(*ClientResponse))
			response, err := srv.handler(srv.resps[resp.broadcastID])
			if err != nil {
				delete(srv.resps, resp.broadcastID)
				srv.doneChan <- response
			}
		case broadcastID := <-srv.reqChan:
			srv.resps[broadcastID] = make([]*ClientResponse, 0)
		}
	}
}
*/
