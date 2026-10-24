package gengorums

// clientVariables qualifies the runtime types that the client types bind to.
var clientVariables = `
{{$config := use "gorums.Config" .GenFile}}
{{$node := use "gorums.Node" .GenFile}}
`

// clientType generates, for each service, the client type bound to a
// configuration and its constructor and target access methods.
var clientType = `
{{range .Services -}}
{{$service := .GoName}}
// {{$service}}Client invokes the configuration-side calls of the {{$service}} service.
// It is bound to a configuration.
type {{$service}}Client struct {
	config {{$config}}
}

// New{{$service}}Client returns a client for the {{$service}} service, bound to config.
func New{{$service}}Client(config {{$config}}) {{$service}}Client {
	return {{$service}}Client{config: config}
}

// Config returns the configuration to which c is bound.
func (c {{$service}}Client) Config() {{$config}} {
	return c.config
}

// WithConfig returns a copy of c bound to config.
func (c {{$service}}Client) WithConfig(config {{$config}}) {{$service}}Client {
	return {{$service}}Client{config: config}
}

// ForNode returns a node client for the {{$service}} service, bound to node.
// The node need not belong to the configuration of c.
func (c {{$service}}Client) ForNode(node *{{$node}}) {{$service}}ClientNode {
	return {{$service}}ClientNode{node: node}
}
{{- end}}
`

// clientNodeType generates, for each service, the client type bound to a
// single node and its constructor and target access method.
var clientNodeType = `
{{range .Services -}}
{{$service := .GoName}}
// {{$service}}ClientNode invokes the node-side calls of the {{$service}} service.
// It is bound to a single node.
type {{$service}}ClientNode struct {
	node *{{$node}}
}

// New{{$service}}ClientNode returns a node client for the {{$service}} service, bound to node.
func New{{$service}}ClientNode(node *{{$node}}) {{$service}}ClientNode {
	return {{$service}}ClientNode{node: node}
}

// Node returns the node to which c is bound.
func (c {{$service}}ClientNode) Node() *{{$node}} {
	return c.node
}
{{- end}}
`

var client = clientVariables + clientType + clientNodeType
