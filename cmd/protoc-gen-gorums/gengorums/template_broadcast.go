package gengorums

var broadcastVar = `
{{$callData := use "gorums.CallData" .GenFile}}
`

var broadcastSignature = `func (b *Broadcast) {{.Method.GoName}}(req *{{in .GenFile .Method}}, opts... gorums.BroadcastOption) {`

var broadcastBody = `
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	b.sp.BroadcastHandler("{{.Method.Desc.FullName}}", req, b.metadata, options)
}
`

var broadcastMethod = broadcastVar +
	broadcastSignature + broadcastBody
