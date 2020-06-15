package gengorums

var unicastMethod = `
func (n *Node) {{$method}}(in *{{$in}}) error {
	msgID := n.nextMsgID()
	data, err := {{$marshalOptions}}{AllowPartial: true, Deterministic: true}.Marshal(in)
	if err != nil {
		return {{$errorf}}("failed to marshal message: %w", err)
	}
	msg := &{{$gorumsMsg}}{
		ID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
		Data: data,
	}
	n.sendQ <- msg
	return nil
}
`

var unicastCall = commonVariables +
	orderingVariables +
	multicastRefImports +
	unicastMethod
