package gengorums

var configurationVars = `
{{- $rawConfiguration := use "gorums.RawConfiguration" .GenFile}}
{{- $nodeListOptions := use "gorums.NodeListOption" .GenFile}}
`

var configurationServicesBegin = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $configurationName := serviceTypeName $service "Configuration"}}
	{{- $nodeName := serviceTypeName $service "Node"}}
`

var configurationServicesEnd = `
{{- end}}
`

var configurationStruct = `
// A {{$configurationName}} represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type {{$configurationName}} struct {
	{{$rawConfiguration}}
}
`

var newConfiguration = `
// New{{$configurationName}} returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed.
// Nodes can be supplied using WithNodeMap or WithNodeList.
// Using any other type of NodeListOption will not work.
// The ManagerOption list controls how the nodes in the configuration are created.
func New{{$configurationName}}(cfg gorums.NodeListOption, opts ...gorums.ManagerOption) (c *{{$configurationName}}, err error) {
	c = &{{$configurationName}}{}
	c.RawConfiguration, err = gorums.NewRawConfiguration(cfg, opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}
`

var subConfiguration = `
// Sub{{$configurationName}} allows for making a new Configuration from the
// ManagerOption list and node list of another set of configurations,
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (c *{{$configurationName}}) Sub{{$configurationName}}(cfg gorums.NodeListOption) (subCfg *{{$configurationName}}, err error) {
	subCfg = &{{$configurationName}}{}
	subCfg.RawConfiguration, err = c.SubRawConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	return subCfg, nil
}
`

var configurationFromRaw = `
// {{$configurationName}}FromRaw returns a new {{$configurationName}} from the given raw configuration.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func {{$configurationName}}FromRaw(rawCfg {{$rawConfiguration}}) (*{{$configurationName}}, error) {
	newCfg := &{{$configurationName}}{
		RawConfiguration: rawCfg,
	}
	return newCfg, nil
}
`

var configurationMethodsTemplate = `
// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
func (c *{{$configurationName}}) Nodes() []*{{$nodeName}} {
	rawNodes := c.RawConfiguration.Nodes()
	nodes := make([]*{{$nodeName}}, len(rawNodes))
	for i, n := range rawNodes {
		nodes[i] = &{{$nodeName}}{n}
	}
	return nodes
}

// AllNodes returns a slice of each available node of all subconfigurations. Sorted by node id.
//
// NOTE: mutating the returned slice is not supported.
func (c *{{$configurationName}}) AllNodes() []*{{$nodeName}} {
	rawNodes := c.RawConfiguration.AllNodes()
	nodes := make([]*{{$nodeName}}, len(rawNodes))
	for i, n := range rawNodes {
		nodes[i] = &{{$nodeName}}{n}
	}
	return nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c {{$configurationName}}) And(d *{{$configurationName}}) {{$nodeListOptions}} {
	return c.RawConfiguration.And(d.RawConfiguration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c {{$configurationName}}) Except(rm *{{$configurationName}}) {{$nodeListOptions}} {
	return c.RawConfiguration.Except(rm.RawConfiguration)
}
`

var configuration = configurationVars +
	configurationServicesBegin +
	configurationStruct + newConfiguration + subConfiguration + configurationFromRaw + configurationMethodsTemplate +
	configurationServicesEnd
