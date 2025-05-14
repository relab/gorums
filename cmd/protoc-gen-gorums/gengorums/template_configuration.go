package gengorums

var configuration = `
{{- $configuration := use "gorums.Configuration" .GenFile}}
{{- $nodeListOptions := use "gorums.NodeListOption" .GenFile}}
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $configurationName := printf "%sConfiguration" $service}}
	{{- $nodeName := printf "%sNode" $service}}
	// A {{$configurationName}} represents a static set of nodes on which quorum remote
	// procedure calls may be invoked.
	{{- reserveName $configurationName}}
	type {{$configurationName}} struct {
		cfg *{{$configuration}}
	}

	{{$funcName := printf "%sRpc" $configurationName}}
	{{- reserveName $funcName}}
	func {{$funcName}}(cfg *{{$configuration}}) {{$configurationName}} {
		return {{$configurationName}}{
			cfg: cfg,
		}
	}
{{- end}}
`
