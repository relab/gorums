package gengorums

var configurationVars = `
{{- $configuration := use "gorums.Configuration" .GenFile}}
{{- $nodeListOptions := use "gorums.NodeListOption" .GenFile}}
`

var configurationServicesBegin = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $configurationName := printf "%sConfiguration" $service}}
	{{- $nodeName := printf "%sNode" $service}}
`

var configurationServicesEnd = `
{{- end}}
`

var configurationStruct = `
// A {{$configurationName}} represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
{{- reserveName $configurationName}}
type {{$configurationName}} struct {
	{{$configuration}}
}
`

var newConfiguration = `
{{- $funcName := printf "New%s" $configurationName}}
// {{$funcName}} returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed.
// Nodes can be supplied using WithNodeMap or WithNodeList.
// Using any other type of NodeListOption will not work.
// The ManagerOption list controls how the nodes in the configuration are created.
{{- reserveName $funcName}}
func {{$funcName}}(cfg gorums.NodeListOption, opts ...gorums.ManagerOption) (c *{{$configurationName}}, err error) {
	c = &{{$configurationName}}{}
	c.Configuration, err = gorums.NewConfiguration(cfg, opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}
`

var subConfiguration = `
{{- $methodName := printf "Sub%s" $configurationName}}
// {{$methodName}} allows for making a new Configuration from the
// ManagerOption list and node list of another set of configurations,
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
{{- reserveMethod $configurationName $methodName}}
func (c *{{$configurationName}}) {{$methodName}}(cfg gorums.NodeListOption) (subCfg *{{$configurationName}}, err error) {
	subCfg = &{{$configurationName}}{}
	subCfg.Configuration, err = c.SubConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	return subCfg, nil
}
`

var configurationFromRaw = `
{{- $funcName := printf "%sFromRaw" $configurationName}}
// {{$funcName}} returns a new {{$configurationName}} from the given raw configuration.
//
// This function may for example be used to "clone" a configuration:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
{{- reserveName $funcName}}
func {{$funcName}}(rawCfg {{$configuration}}) (*{{$configurationName}}, error) {
	newCfg := &{{$configurationName}}{
		Configuration: rawCfg,
	}
	return newCfg, nil
}
`

var configurationMethodsTemplate = `
// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
{{- reserveMethod $configurationName "Nodes"}}
func (c *{{$configurationName}}) Nodes() []*{{$nodeName}} {
	rawNodes := c.Configuration.Nodes()
	nodes := make([]*{{$nodeName}}, len(rawNodes))
	for i, n := range rawNodes {
		nodes[i] = &{{$nodeName}}{n}
	}
	return nodes
}

// AllNodes returns a slice of each available node of all subconfigurations. Sorted by node id.
//
// NOTE: mutating the returned slice is not supported.
{{- reserveMethod $configurationName "AllNodes"}}
func (c *{{$configurationName}}) AllNodes() []*{{$nodeName}} {
	rawNodes := c.Configuration.AllNodes()
	nodes := make([]*{{$nodeName}}, len(rawNodes))
	for i, n := range rawNodes {
		nodes[i] = &{{$nodeName}}{n}
	}
	return nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
{{- reserveMethod $configurationName "And"}}
func (c {{$configurationName}}) And(d *{{$configurationName}}) {{$nodeListOptions}} {
	return c.Configuration.And(d.Configuration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
{{- reserveMethod $configurationName "Except"}}
func (c {{$configurationName}}) Except(rm *{{$configurationName}}) {{$nodeListOptions}} {
	return c.Configuration.Except(rm.Configuration)
}
`

var configuration = configurationVars +
	configurationServicesBegin +
	configurationStruct + newConfiguration + subConfiguration + configurationFromRaw + configurationMethodsTemplate +
	configurationServicesEnd
