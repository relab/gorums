package gengorums

var configurationVars = `
{{$rawConfiguration := use "gorums.RawConfiguration" .GenFile}}
{{$nodeListOptions := use "gorums.NodeListOption" .GenFile}}
`

var configurationServicesBegin = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $configurationName := printf "%sConfiguration" $service}}
	{{- $qspecName := printf "%sQuorumSpec" $service}}
	{{- $nodeName := printf "%sNode" $service}}
	{{- $isQspec := ne (len (qspecMethods .Methods))	0}}
`

var configurationServicesEnd = `
{{- end}}
`

var configurationStruct = `
// A {{$configurationName}} represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type {{$configurationName}} struct {
	{{$rawConfiguration}}
	{{- if $isQspec}}
		qspec {{$qspecName}}
	{{- end}}
	nodes []*{{$nodeName}}
}
`

var configurationFromRaw = `
// {{$qspecName}}FromRaw returns a new {{$qspecName}} from the given raw configuration{{if $isQspec}} and QuorumSpec{{end}}.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func {{$configurationName}}FromRaw(rawCfg {{$rawConfiguration}}{{if $isQspec}}, qspec {{$qspecName}}{{end}}) (*{{$configurationName}}, error) {
	{{- if $isQspec}}
		// return an error if qspec is nil.
		if qspec == nil {
			return nil, {{use "fmt.Errorf" $genFile}}("config: missing required QuorumSpec")
		}
	{{- end}}
	newCfg := &{{$configurationName}}{
		RawConfiguration: rawCfg,
		{{- if $isQspec}}
			qspec:            qspec,
		{{- end}}
	}
	// initialize the nodes slice
	newCfg.nodes = make([]*{{$nodeName}}, newCfg.Size())
	for i, n := range rawCfg {
		newCfg.nodes[i] = &{{$nodeName}}{n}
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
	return c.nodes
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
	configurationStruct + configurationFromRaw + configurationMethodsTemplate +
	configurationServicesEnd
