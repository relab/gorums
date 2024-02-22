package gengorums

var clientServerVar = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$grpc := use "grpc.GRPC" .GenFile}}
{{$uuid := use "uuid.UUID" .GenFile}}
{{$fmt := use "fmt.FMT" .GenFile}}
{{$protoMessage := use "protoreflect.ProtoMessage" .GenFile}}
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
		return nil, fmt.Errorf("a client server is not defined. Use configuration.RegisterClientServer() to define a client server")
	}
	if c.replySpec == nil {
		return nil, fmt.Errorf("a reply spec is not defined. Use configuration.RegisterClientServer() to define a reply spec")
	}
	broadcastID := uuid.New().String()
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "protos.UniformBroadcast.Broadcast",

		BroadcastID: broadcastID,
		Sender:      "client",
		OriginAddr: c.listenAddr,
	}
	doneChan := make(chan protoreflect.ProtoMessage)
	c.srv.reqChan <- &clientRequest{
		broadcastID: broadcastID,
		doneChan: doneChan,
		handler: convertToType[*{{out .GenFile .Method}}](c.replySpec.{{.Method.GoName}}),
	}
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	response, ok := <-doneChan
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	return response.(*{{out .GenFile .Method}}), err
}
`

var clientServerCall = clientServerVar +
	clientServerSignature + clientServerBody + clientServerMethodImpl + clientServerImplMethod
