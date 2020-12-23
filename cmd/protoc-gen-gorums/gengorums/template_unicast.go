package gengorums

var unicastVar = rpcVar + `{{$callOpt := use "gorums.CallOption" .GenFile}}`

var unicastSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, opts ...{{$callOpt}}) {
`

var unicastBody = `
	cd := {{$callData}}{
		Manager:  n.mgr.Manager,
		Node:     n.Node,
		Message:  in,
		MethodID: {{$unexportMethod}}MethodID,
	}

	{{use "gorums.Unicast" $genFile}}(ctx, cd, opts...)
}
`

var unicastCall = commonVariables +
	unicastVar +
	multicastRefImports +
	quorumCallComment +
	unicastSignature +
	unicastBody
