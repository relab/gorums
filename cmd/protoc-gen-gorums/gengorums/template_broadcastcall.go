package gengorums

var clientServerVar = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$grpc := use "grpc.GRPC" .GenFile}}
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
func (srv *clientServerImpl) client{{.Method.GoName}}(ctx context.Context, resp *{{out .GenFile .Method}}) (*{{out .GenFile .Method}}, error) {
	err := srv.AddResponse(ctx, resp)
	return resp, err
}

`

var clientServerImplMethod = `
func (c *Configuration) {{.Method.GoName}}(ctx context.Context, in *{{in .GenFile .Method}}) (resp *{{out .GenFile .Method}}, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("a client server is not defined. Use configuration.RegisterClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined.")
	}
	doneChan, cd := c.srv.AddRequest(ctx, in, gorums.ConvertToType(c.qspec.{{.Method.GoName}}QF))
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	response, ok := <-doneChan
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	return response.(*{{out .GenFile .Method}}), err
}
`

var broadcastCall = clientServerVar +
	clientServerSignature + clientServerBody + clientServerMethodImpl + clientServerImplMethod
