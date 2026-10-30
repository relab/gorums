package gengorums

import (
	"fmt"
	"strings"

	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

// callTypeInfo describes how to generate code for one call type.
type callTypeInfo struct {
	// template generates one client method of this call type. It is empty if
	// nested holds the templates.
	template string
	// client is the client interface that holds the methods of this call type.
	// It is nil if nested holds the variants.
	client *clientKind
	// oneway reports whether the call type has no reply. Its generated code
	// may leave the package of the output type unused.
	oneway bool
	// match reports whether a method has this call type.
	match func(m *protogen.Method) bool
	// nested holds the variants of this call type, keyed by variant name.
	// The variant whose match accepts a method replaces this entry for that
	// method. The variants of an entry are mutually exclusive.
	nested map[string]*callTypeInfo
}

// matches reports whether the given method has this call type.
func (c *callTypeInfo) matches(m *protogen.Method) bool {
	if c != nil && c.match != nil {
		return c.match(m)
	}
	return false
}

// resolve returns the nested call type that matches the given method,
// or c if no nested call type matches.
func (c *callTypeInfo) resolve(m *protogen.Method) *callTypeInfo {
	if c != nil {
		for _, nested := range c.nested {
			if nested.match(m) {
				return nested
			}
		}
	}
	return c
}

// callTypeName returns the name of the call type option with the prefix removed.
func callTypeName(ext *protoimpl.ExtensionInfo) string {
	s := string(ext.TypeDescriptor().FullName())
	return s[strings.LastIndex(s, ".")+1:]
}

// Names of the variants of the quorum call type.
const (
	quorumCallVariant       = "quorumcall"
	quorumCallStreamVariant = "quorumcall_stream"
)

// gorumsCallTypesInfo maps each Gorums call type name to its [callTypeInfo].
// The entries are mutually exclusive: at most one matches a given method. In
// dev mode, each key names a generated zorums_{key}_gorums.pb.go file.
var gorumsCallTypesInfo = map[string]*callTypeInfo{
	callTypeName(gorums.E_Remotecall): {
		template: remoteCall,
		client:   nodeClientKind,
		match: func(m *protogen.Method) bool {
			return !hasMethodOption(m, gorumsCallTypes...)
		},
	},
	callTypeName(gorums.E_Quorumcall): {
		match: func(m *protogen.Method) bool {
			return hasMethodOption(m, gorums.E_Quorumcall)
		},
		nested: map[string]*callTypeInfo{
			quorumCallVariant: {
				template: quorumCall,
				client:   configClientKind,
				match: func(m *protogen.Method) bool {
					return hasMethodOption(m, gorums.E_Quorumcall) && !m.Desc.IsStreamingServer()
				},
			},
			quorumCallStreamVariant: {
				template: quorumCallStream,
				client:   configClientKind,
				match: func(m *protogen.Method) bool {
					return hasMethodOption(m, gorums.E_Quorumcall) && m.Desc.IsStreamingServer()
				},
			},
		},
	},
	callTypeName(gorums.E_Multicast): {
		template: multicastCall,
		client:   configClientKind,
		oneway:   true,
		match: func(m *protogen.Method) bool {
			return hasMethodOption(m, gorums.E_Multicast)
		},
	},
	callTypeName(gorums.E_Unicast): {
		template: unicastCall,
		client:   nodeClientKind,
		oneway:   true,
		match: func(m *protogen.Method) bool {
			return hasMethodOption(m, gorums.E_Unicast)
		},
	},
}

// callType returns the name and information of the call type that matches
// the given method. The name is the key in [gorumsCallTypesInfo]. If that
// entry has a nested call type that matches the method, the returned
// information is the nested entry's. It returns "" and nil if no call type
// matches.
func callType(method *protogen.Method) (string, *callTypeInfo) {
	for name, info := range gorumsCallTypesInfo {
		if info = info.resolve(method); info.matches(method) {
			return name, info
		}
	}
	return "", nil
}

// hasGorumsMethods returns true if one of the methods in the set of services
// has a Gorums call type.
func hasGorumsMethods(services []*protogen.Service) bool {
	for _, service := range services {
		for _, method := range service.Methods {
			if _, info := callType(method); info != nil {
				return true
			}
		}
	}
	return false
}

// gorumsCallTypes should list all available call types supported by Gorums.
// A method may have at most one of them; [validateOptions] rejects more.
var gorumsCallTypes = []*protoimpl.ExtensionInfo{
	gorums.E_Quorumcall,
	gorums.E_Multicast,
	gorums.E_Unicast,
}

// hasMethodOption reports whether the method has one of the given method options.
func hasMethodOption(method *protogen.Method, exts ...*protoimpl.ExtensionInfo) bool {
	options := protoimpl.X.MessageOf(method.Desc.Options()).Interface()
	for _, ext := range exts {
		if proto.HasExtension(options, ext) {
			return true
		}
	}
	return false
}

// validateOptions returns an error if the extension options
// for the provided method are incompatible.
func validateOptions(method *protogen.Method) error {
	var set []*protoimpl.ExtensionInfo
	for _, ext := range gorumsCallTypes {
		if hasMethodOption(method, ext) {
			set = append(set, ext)
		}
	}
	switch {
	case len(set) > 1:
		return fmt.Errorf("%s.%s: options '%s' and '%s' cannot be combined", method.Parent.Desc.Name(), method.Desc.Name(),
			set[0].TypeDescriptor().FullName(), set[1].TypeDescriptor().FullName())

	case !hasMethodOption(method, gorums.E_Multicast) && method.Desc.IsStreamingClient():
		return optionErrorf("is required for client-server stream methods", method, gorums.E_Multicast)

	case !hasMethodOption(method, gorums.E_Quorumcall) && method.Desc.IsStreamingServer():
		return optionErrorf("is required for server-client stream methods", method, gorums.E_Quorumcall)
	}
	return nil
}

func optionErrorf(s string, method *protogen.Method, ext *protoimpl.ExtensionInfo) error {
	return fmt.Errorf("%s.%s: option '%s' "+s, method.Parent.Desc.Name(), method.Desc.Name(), ext.TypeDescriptor().FullName())
}
