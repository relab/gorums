package gengorums

var broadcastVar = `
{{$callData := use "gorums.CallData" .GenFile}}
`

var broadcastSignature = `func (b *Broadcast) {{.Method.GoName}}(req *{{in .GenFile .Method}}, opts... gorums.BroadcastOption) {`

var broadcastBody = `
	data := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&data)
	}
	b.sp.BroadcastHandler("{{.Method.Desc.FullName}}", req, b.metadata, data)
}
`

var broadcastMethod = broadcastVar +
	broadcastSignature + broadcastBody
