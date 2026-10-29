package gengorums

import (
	"strings"

	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

// clientKind describes one of the two client interfaces of a service:
// the configuration client and the node client.
type clientKind struct {
	// suffix follows the service's Go name in the interface name.
	suffix string
	// template generates the interface, its constructor, and its
	// implementing struct.
	template string
	// has reports whether a method belongs to this client.
	has func(*protogen.Method) bool
}

// clientKinds lists the client interfaces that each service may have.
// Every Gorums method belongs to exactly one of them.
var clientKinds = []clientKind{
	{suffix: "Client", template: configClient, has: isConfigCall},
	{suffix: "NodeClient", template: nodeClient, has: isNodeCall},
}

// isNodeCall reports whether the method is a remote call or a unicast, which
// executes on one node.
func isNodeCall(method *protogen.Method) bool {
	name, _ := callType(method)
	return name == callTypeName(gorums.E_Remotecall) || name == callTypeName(gorums.E_Unicast)
}

// isConfigCall reports whether the method is a quorum call or a multicast,
// which executes on a configuration.
func isConfigCall(method *protogen.Method) bool {
	name, _ := callType(method)
	return name == callTypeName(gorums.E_Quorumcall) || name == callTypeName(gorums.E_Multicast)
}

// methods returns the service's methods that belong to this client.
func (k clientKind) methods(service *protogen.Service) []*protogen.Method {
	var methods []*protogen.Method
	for _, method := range service.Methods {
		if k.has(method) {
			methods = append(methods, method)
		}
	}
	return methods
}

// clientIdents returns the package-level identifiers that [genClients]
// declares for the given service: the name, constructor, and implementing
// struct of each client interface that the service has.
func clientIdents(service *protogen.Service) []string {
	var idents []string
	for _, kind := range clientKinds {
		if len(kind.methods(service)) == 0 {
			continue
		}
		name := service.GoName + kind.suffix
		idents = append(idents, name, "New"+name, unexport(service.GoName)+kind.suffix)
	}
	return idents
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
			data := clientData{GenFile: g, Service: service, Methods: strings.Join(decls, "\n\n")}
			g.P(mustExecute(parseTemplate(kind.suffix, kind.template), data))
		}
	}
}

// configClient generates the configuration client of a service.
var configClient = `
{{$config := use "gorums.Config" .GenFile}}
{{$service := .Service.GoName}}
{{$impl := printf "%sClient" (unexport $service)}}
// {{$service}}Client is the client API for the quorum calls and multicasts of
// the {{$service}} service. Its calls execute on the configuration given to
// [New{{$service}}Client]. A call on an empty configuration panics when it is
// built.
type {{$service}}Client interface {
{{.Methods}}
}

// New{{$service}}Client returns a client whose calls execute on config.
func New{{$service}}Client(config {{$config}}) {{$service}}Client {
	return {{$impl}}{config: config}
}

// {{$impl}} implements {{$service}}Client.
type {{$impl}} struct {
	config {{$config}}
}
`

// nodeClient generates the node client of a service.
var nodeClient = `
{{$node := use "gorums.Node" .GenFile}}
{{$errNoNode := use "gorums.ErrNoNode" .GenFile}}
{{$service := .Service.GoName}}
{{$impl := printf "%sNodeClient" (unexport $service)}}
// {{$service}}NodeClient is the client API for the remote calls and unicasts
// of the {{$service}} service. Its calls execute on the node given to
// [New{{$service}}NodeClient]. If that node is nil, every call returns
// [{{$errNoNode}}].
type {{$service}}NodeClient interface {
{{.Methods}}
}

// New{{$service}}NodeClient returns a node client whose calls execute on node.
// The node may be nil, and it need not belong to a particular configuration.
func New{{$service}}NodeClient(node *{{$node}}) {{$service}}NodeClient {
	return {{$impl}}{node: node}
}

// {{$impl}} implements {{$service}}NodeClient.
type {{$impl}} struct {
	node *{{$node}}
}
`
