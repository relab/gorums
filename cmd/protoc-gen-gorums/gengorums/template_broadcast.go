package gengorums

var broadcastSignature = `func (b *Broadcast) {{.Method.GoName}}(req *{{in .GenFile .Method}}) {`

var broadcastBody = `	b.SetBroadcastValues("{{.Method.Desc.FullName}}", req)
}`

var broadcastCall = broadcastSignature + broadcastBody
