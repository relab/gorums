package mock

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func init() {
	err := RegisterServices([]Service{
		{
			Name: "mock.MockService",
			Methods: []Method{
				{Name: "Test", Input: &pb.StringValue{}, Output: &pb.StringValue{}},
				{Name: "GetValue", Input: &pb.Int32Value{}, Output: &pb.Int32Value{}},
				{Name: "Stream", Input: &pb.StringValue{}, Output: &pb.StringValue{}},
			},
		},
	})
	if err != nil {
		panic(err)
	}
}

// TestMethod and GetValueMethod are the methods supported by the mock package.
const (
	TestMethod     = "mock.MockService.Test"
	GetValueMethod = "mock.MockService.GetValue"
	Stream         = "mock.MockService.Stream"
)

// Service represents a service to be registered.
type Service struct {
	Name    string // Full package and service name, e.g., "mock.MockService"
	Methods []Method
}

// Method represents a method in a service.
type Method struct {
	Name   string
	Input  proto.Message
	Output proto.Message
}

// RegisterServices registers the given services in the global registry.
// It is safe to call multiple times, but services with the same package name
// must be registered in the same call or be identical to previous registrations.
// Returns an error if registration fails.
func RegisterServices(services []Service) error {
	// Group by package
	packages := make(map[string][]*descriptorpb.ServiceDescriptorProto)

	for _, s := range services {
		pkgName, svcName, found := strings.Cut(s.Name, ".")
		if !found {
			return fmt.Errorf("service name %q must contain a package", s.Name)
		}

		svcDesc := &descriptorpb.ServiceDescriptorProto{
			Name: proto.String(svcName),
		}

		for _, m := range s.Methods {
			inDesc := m.Input.ProtoReflect().Descriptor()
			outDesc := m.Output.ProtoReflect().Descriptor()
			inName := string(inDesc.FullName())
			outName := string(outDesc.FullName())

			svcDesc.Method = append(svcDesc.Method, &descriptorpb.MethodDescriptorProto{
				Name:       proto.String(m.Name),
				InputType:  proto.String("." + inName),
				OutputType: proto.String("." + outName),
			})
		}
		packages[pkgName] = append(packages[pkgName], svcDesc)
	}

	for pkg, svcDescriptors := range packages {
		// Collect dependencies
		deps := make(map[string]struct{})

		// Iterate over the original services to find dependencies for this package.
		for _, s := range services {
			pName, _, found := strings.Cut(s.Name, ".")
			if !found || pName != pkg {
				continue
			}

			for _, m := range s.Methods {
				if d := m.Input.ProtoReflect().Descriptor().ParentFile(); d != nil {
					deps[d.Path()] = struct{}{}
				}
				if d := m.Output.ProtoReflect().Descriptor().ParentFile(); d != nil {
					deps[d.Path()] = struct{}{}
				}
			}
		}

		fd := &descriptorpb.FileDescriptorProto{
			Name:    proto.String(fmt.Sprintf("mock/%s.proto", pkg)),
			Package: proto.String(pkg),
			Service: svcDescriptors,
		}

		for dep := range deps {
			fd.Dependency = append(fd.Dependency, dep)
		}

		// Check if already registered
		if _, err := protoregistry.GlobalFiles.FindFileByPath(fd.GetName()); err == nil {
			continue // Already registered
		}

		fileDesc, err := protodesc.NewFile(fd, protoregistry.GlobalFiles)
		if err != nil {
			return fmt.Errorf("failed to create file descriptor for %s: %v", pkg, err)
		}

		if err := protoregistry.GlobalFiles.RegisterFile(fileDesc); err != nil {
			return fmt.Errorf("failed to register file %s: %v", pkg, err)
		}
	}
	return nil
}

// Helpers for Mock messages

func GetVal(msg proto.Message) string {
	if msg == nil {
		return ""
	}
	if m, ok := msg.(*pb.StringValue); ok {
		return m.Value
	}
	return ""
}

func SetVal(msg proto.Message, val string) {
	if msg == nil {
		return
	}
	if m, ok := msg.(*pb.StringValue); ok {
		m.Value = val
	}
}
