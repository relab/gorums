package gengorums

var broadcastVar = `
{{$callData := use "gorums.CallData" .GenFile}}
`

var broadcastSignature = `func (b *Broadcast) {{.Method.GoName}}(req *{{in .GenFile .Method}}) {`

var broadcastBody = `	b.sp.BroadcastHandler("{{.Method.Desc.FullName}}", req, b.metadata)
}

`

var optSignature = `func (bd *broadcastData) {{.Method.GoName}}(req *{{in .GenFile .Method}}) {`

var optBody = `
	data := bd.data
	bd.mu.Unlock()
	bd.b.sp.BroadcastHandler("{{.Method.Desc.FullName}}", req, bd.b.metadata, data)
}`

var broadcastCall = broadcastVar +
	broadcastSignature + broadcastBody + optSignature + optBody
