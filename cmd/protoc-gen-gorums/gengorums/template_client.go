package gengorums

import "google.golang.org/protobuf/compiler/protogen"

// clientHelpers lists the methods that [clientType] declares on SClient
// besides the RPC methods.
var clientHelpers = []string{"Config"}

// clientIdents returns the package-level identifiers that [client] declares
// for the given service.
func clientIdents(service *protogen.Service) []string {
	client := service.GoName + "Client"
	return []string{client, "New" + client}
}

// clientVariables qualifies the runtime types that the client type binds to.
var clientVariables = `
{{$config := use "gorums.Config" .GenFile}}
`

// clientType generates, for each service, the client type bound to a
// configuration, its constructor, and its configuration access method.
var clientType = `
{{range .Services -}}
{{$service := .GoName}}
// {{$service}}Client invokes the calls of the {{$service}} service.
// Quorum calls and multicasts execute on the client's configuration.
// Remote calls and unicasts execute on the node given to each call; that node
// need not belong to the client's configuration.
// The zero value is ready for remote calls and unicasts.
// Quorum calls and multicasts need a configuration from
// [New{{$service}}Client]; they panic on an empty configuration.
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
{{- end}}
`

var client = clientVariables + clientType
