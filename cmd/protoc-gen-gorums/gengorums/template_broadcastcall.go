package gengorums

var clientServerVar = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$fmt := use "fmt.FMT" .GenFile}}
{{$protoMessage := use "protoreflect.ProtoMessage" .GenFile}}
`

var clientServerMethodImpl = `
func (srv *clientServerImpl) client{{.Method.GoName}}(ctx context.Context, resp *{{out .GenFile .Method}}, broadcastID uint64) (*{{out .GenFile .Method}}, error) {
	err := srv.AddResponse(ctx, resp, broadcastID)
	return resp, err
}

`

var clientServerImplMethod = `
func (c *Configuration) {{.Method.GoName}}(ctx context.Context, in *{{in .GenFile .Method}}) (resp *{{out .GenFile .Method}}, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("config: a client server is not defined. Use mgr.AddClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	doneChan, cd := c.srv.AddRequest(c.snowflake.NewBroadcastID(), ctx, in, gorums.ConvertToType(c.qspec.{{.Method.GoName}}QF), "{{.Method.Desc.FullName}}")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	var response {{$protoMessage}}
	var ok bool
	select {
	case response, ok = <-doneChan:
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	}
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	resp, ok = response.(*{{out .GenFile .Method}})
	if !ok {
		return nil, fmt.Errorf("wrong proto format")
	}
	return resp, nil
}
`

var broadcastCall = clientServerVar + clientServerMethodImpl + clientServerImplMethod
