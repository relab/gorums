package gengorums

var unicastSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}) {
`

var unicastBody = `
	cd := {{$callData}}{
		Manager:  n.mgr.Manager,
		Node:     n.Node,
		Message:  in,
		MethodID: {{$unexportMethod}}MethodID,
	}

	{{use "gorums.Unicast" $genFile}}(ctx, cd)
}
`

var unicastCall = commonVariables +
	rpcVar +
	multicastRefImports +
	quorumCallComment +
	unicastSignature +
	unicastBody
