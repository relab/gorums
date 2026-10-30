package gengorums

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// Name parts of the identifiers that Gorums declares for a service.
const (
	configClientSuffix = "Client"
	nodeClientSuffix   = "NodeClient"
	serverSuffix       = "Server"
	constructorPrefix  = "New"
	registerPrefix     = "Register"
)

// clientKind describes one of the two client interfaces of a service:
// the configuration client and the node client.
type clientKind struct {
	// suffix follows the service's Go name in the interface name.
	suffix string
	// template generates the interface, its constructor, and its
	// implementing struct.
	template string
}

// The client interfaces that each service may have. Every Gorums method
// belongs to exactly one of them, as its call type in [gorumsCallTypesInfo]
// says.
var (
	configClientKind = &clientKind{suffix: configClientSuffix, template: configClient}
	nodeClientKind   = &clientKind{suffix: nodeClientSuffix, template: nodeClient}
	clientKinds      = []*clientKind{configClientKind, nodeClientKind}
)

// methods returns the service's methods that belong to this client.
func (k *clientKind) methods(service *protogen.Service) []*protogen.Method {
	var methods []*protogen.Method
	for _, method := range service.Methods {
		if _, info := callType(method); info != nil && info.client == k {
			methods = append(methods, method)
		}
	}
	return methods
}

// clientNames holds the package-level identifiers that [genClients] declares
// for one client interface of a service. The client template takes them as
// data, so that the templates and [serviceIdents] agree on the names.
type clientNames struct {
	// Interface is the name of the client interface.
	Interface string
	// Constructor is the name of the function that returns the client.
	Constructor string
	// Impl is the name of the struct that implements the interface.
	Impl string
}

// clientNamesOf returns the identifiers that [genClients] declares for the
// given client interface of service.
func clientNamesOf(service *protogen.Service, kind *clientKind) clientNames {
	name := service.GoName + kind.suffix
	return clientNames{
		Interface:   name,
		Constructor: constructorPrefix + name,
		Impl:        unexport(service.GoName) + kind.suffix,
	}
}

// idents returns the names as a list.
func (n clientNames) idents() []string {
	return []string{n.Interface, n.Constructor, n.Impl}
}

// genClients generates, for each service, the client interfaces that the
// service has, each with its method declarations, constructor, and
// implementing struct.
func genClients(g *protogen.GeneratedFile, services []*protogen.Service) {
	// clientData is the data of a client template. Methods holds the
	// rendered method declarations of the interface.
	type clientData struct {
		GenFile *protogen.GeneratedFile
		Service *protogen.Service
		Names   clientNames
		Methods string
	}
	for _, service := range services {
		for _, kind := range clientKinds {
			methods := kind.methods(service)
			if len(methods) == 0 {
				continue
			}
			decls := make([]string, len(methods))
			for i, method := range methods {
				decls[i] = strings.TrimSpace(renderMethod(g, method, true))
			}
			data := clientData{g, service, clientNamesOf(service, kind), strings.Join(decls, "\n\n")}
			g.P(mustExecute(parseTemplate(kind.suffix, kind.template), data))
		}
	}
}

// configClient generates the configuration client of a service. Config is the
// alias that the package file declares.
var configClient = `
{{$errEmptyConfig := use "gorums.ErrEmptyConfig" .GenFile}}
{{$service := .Service.GoName}}
// {{.Names.Interface}} is the client API for the quorum calls and multicasts of
// the {{$service}} service. Its calls execute on the configuration given to
// [{{.Names.Constructor}}]. If that configuration is empty, every call returns
// [{{$errEmptyConfig}}].
type {{.Names.Interface}} interface {
{{.Methods}}
}

// {{.Names.Constructor}} returns a client whose calls execute on config.
func {{.Names.Constructor}}(config Config) {{.Names.Interface}} {
	return {{.Names.Impl}}{config: config}
}

// {{.Names.Impl}} implements {{.Names.Interface}}.
type {{.Names.Impl}} struct {
	config Config
}
`

// nodeClient generates the node client of a service. Node is the alias that
// the package file declares.
var nodeClient = `
{{$errNoNode := use "gorums.ErrNoNode" .GenFile}}
{{$service := .Service.GoName}}
// {{.Names.Interface}} is the client API for the remote calls and unicasts
// of the {{$service}} service. Its calls execute on the node given to
// [{{.Names.Constructor}}]. If that node is nil, every call returns
// [{{$errNoNode}}].
type {{.Names.Interface}} interface {
{{.Methods}}
}

// {{.Names.Constructor}} returns a node client whose calls execute on node.
// The node may be nil, and it need not belong to a particular configuration.
func {{.Names.Constructor}}(node *Node) {{.Names.Interface}} {
	return {{.Names.Impl}}{node: node}
}

// {{.Names.Impl}} implements {{.Names.Interface}}.
type {{.Names.Impl}} struct {
	node *Node
}
`
