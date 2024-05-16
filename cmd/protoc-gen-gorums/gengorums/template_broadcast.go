package gengorums

var broadcastVar = `
{{$callData := use "gorums.CallData" .GenFile}}
`

var broadcastSignature = `func (b *Broadcast) {{.Method.GoName}}(req *{{in .GenFile .Method}}, opts... gorums.BroadcastOption) {`

var broadcastBody = `
	if b.metadata.BroadcastID == 0 {
		panic("broadcastID cannot be empty. Use srv.Broadcast{{.Method.GoName}} instead")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	options.ServerAddresses = append(options.ServerAddresses, b.srvAddrs...)
	b.orchestrator.BroadcastHandler("{{.Method.Desc.FullName}}", req, b.metadata.BroadcastID, b.enqueueBroadcast, options)
}
`

var broadcastMethod = broadcastVar +
	broadcastSignature + broadcastBody
