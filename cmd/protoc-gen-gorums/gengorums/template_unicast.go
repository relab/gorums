package gengorums

var unicastVar = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$genFile := .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
`

var unicastSignature = `func (n *Node) {{$method}}(` +
	`in *{{$in}}) {
`

var unicastBody = `
	cd := {{$callData}}{
		Manager:  n.mgr.Manager,
		Node:     n.Node,
		Message:  in,
		MethodID: {{$unexportMethod}}MethodID,
	}

	{{use "gorums.Unicast" $genFile}}(cd)
}
`

var unicastCall = commonVariables +
	unicastVar +
	multicastRefImports +
	quorumCallComment +
	unicastSignature +
	unicastBody
