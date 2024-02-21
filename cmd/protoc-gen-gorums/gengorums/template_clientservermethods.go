package gengorums

var clientServerVar = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$grpc := use "grpc.GRPC" .GenFile}}
{{$uuid := use "uuid.UUID" .GenFile}}
{{$fmt := use "fmt.FMT" .GenFile}}
`

var clientServerSignature = `func _client{{.Method.GoName}}(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {`

var clientServerBody = `
	in := new({{out .GenFile .Method}})
	if err := dec(in); err != nil {
		return nil, err
	}
	return srv.(clientServer).client{{.Method.GoName}}(ctx, in)
}

`

var clientServerMethodImpl = `
func (srv *clientServerImpl) client{{.Method.GoName}}(ctx context.Context, resp *{{out .GenFile .Method}}) (any, error) {
	srv.respChan <- &clientResponse{
		broadcastID: ctx.Value("broadcastID").(string),
		data: resp,
	}
	return nil, nil
}

`

var clientServerImplMethod = `
func (c *Configuration) {{.Method.GoName}}(ctx context.Context, in *{{in .GenFile .Method}}) (resp *{{out .GenFile .Method}}, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("a client handler is not defined. Use configuration.RegisterClientHandler() to define a handler")
	}
	broadcastID := uuid.New().String()
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "protos.UniformBroadcast.Broadcast",

		BroadcastID: broadcastID,
		Sender:      "client",
		OriginAddr: c.listenAddr,
	}
	c.srv.reqChan <- broadcastID
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	resp = <-c.srv.doneChan{{.Method.GoName}}
	return resp, err
}
`

var clientServerCall = clientServerVar +
	clientServerSignature + clientServerBody + clientServerMethodImpl + clientServerImplMethod
