package gengorums

var unicastMethod = `
func (n *Node) {{$method}}(in *{{$in}}) error {
	msgID := n.nextMsgID()
	metadata := &{{$gorumsMD}}{
		MessageID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
	}
	msg := &gorumsMessage{metadata: metadata, message: in}
	n.sendQ <- msg
	return nil
}
`

var unicastCall = commonVariables +
	orderingVariables +
	multicastRefImports +
	unicastMethod
