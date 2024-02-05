package gengorums

var broadcastVar = `
{{$callData := use "gorums.CallData" .GenFile}}
`

var broadcastSignature = `func (b *Broadcast) {{.Method.GoName}}(req *{{in .GenFile .Method}}, serverAddresses... string) {`

var broadcastBody = `	b.SetBroadcastValues("{{.Method.Desc.FullName}}", req, serverAddresses...)
}`

var broadcastCall = broadcastVar +
	broadcastSignature + broadcastBody
