package gengorums

var broadcastVar = `
{{$callData := use "gorums.CallData" .GenFile}}
`

var broadcastSignature = `func (b *Broadcast) {{.Method.GoName}}(req *{{in .GenFile .Method}}) {`

var broadcastBody = `	b.sp.BroadcastHandler("{{.Method.Desc.FullName}}", req, b.metadata, b.serverAddresses)
}`

var broadcastCall = broadcastVar +
	broadcastSignature + broadcastBody
