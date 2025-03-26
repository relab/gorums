package gengorums

// gorums need to be imported in the zorums file
var managerVariables = `
{{$_ := use "gorums.EnforceVersion" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var managerStructs = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $managerName := printf "%sManager" $service}}
	// {{$managerName}} maintains a connection pool of nodes on
	// which quorum calls can be performed.
	type {{$managerName}} struct {
		*gorums.RawManager
	}
{{- end}}
`

var managerConstructor = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $managerName := printf "%sManager" $service}}
	// New{{$managerName}} returns a new {{$managerName}} for managing connection to nodes added
	// to the manager. This function accepts manager options used to configure
	// various aspects of the manager.
	func New{{$managerName}}(opts ...gorums.ManagerOption) *{{$managerName}} {
		return &{{$managerName}}{
			RawManager: gorums.NewRawManager(opts...),
		}
	}
{{- end}}
`

var managerNewConfiguration = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $managerName := printf "%sManager" $service}}
	{{- $configurationName := printf "%sConfiguration" $service}}
	{{- $qspecName := printf "%sQuorumSpec" $service}}
	{{- $nodeName := printf "%sNode" $service}}
	{{- $isQspec := ne (len (qspecMethods .Methods))	0}}

	// New{{$configurationName}} returns a {{$configurationName}} based on the provided list of nodes (required) {{- if $isQspec}}and a quorum specification{{- end}}.
	// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
	// A new configuration can also be created from an existing configuration,
	// using the And, WithNewNodes, Except, and WithoutNodes methods.
	func (m *{{$managerName}}) NewConfiguration(cfg gorums.NodeListOption{{if $isQspec}}, qspec {{$qspecName}}{{end}}) (c *{{$configurationName}}, err error) {
		c = &{{$configurationName}}{}
		c.RawConfiguration, err = gorums.NewRawConfiguration(m.RawManager, cfg)
		if err != nil {
			return nil, err
		}
		{{- if $isQspec}}
			// return an error if qspec is nil.
			if qspec == nil {
				return nil, {{use "fmt.Errorf" $genFile}}("config: missing required {{$qspecName}}")
			}
			c.qspec = qspec
		{{- end}}
		// initialize the nodes slice
		c.nodes = make([]*{{$nodeName}}, c.Size())
		for i, n := range c.RawConfiguration {
			c.nodes[i] = &{{$nodeName}}{n}
		}
		return c, nil
	}
{{- end}}
`

var managerMethodNodes = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $managerName := printf "%sManager" $service}}
	{{- $nodeName := printf "%sNode" $service}}
	// Nodes returns a slice of available nodes on this manager.
	// IDs are returned in the order they were added at creation of the manager.
	func (m *{{$managerName}}) Nodes() []*{{$nodeName}} {
		gorumsNodes := m.RawManager.Nodes()
		nodes := make([]*{{$nodeName}}, len(gorumsNodes))
		for i, n := range gorumsNodes {
			nodes[i] = &{{$nodeName}}{n}
		}
		return nodes
	}
{{- end}}
`

var manager = managerVariables + managerStructs + managerConstructor + managerNewConfiguration + managerMethodNodes
