// Package mock registers a mock service descriptor in the global protobuf
// registry, so tests can call its methods through the Gorums runtime without
// a generated service definition. It also provides test doubles and helpers
// shared by the tests of several packages.
//
// Package mock must not import internal/stream, internal/conn, or gorums,
// because package-internal tests of those packages import it. For the same
// reason, those tests cannot use package gorumstest, which imports gorums.
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
	err := registerServices([]service{
		{
			Name: "mock.MockService",
			Methods: []method{
				{Name: "Test", Input: &pb.StringValue{}, Output: &pb.StringValue{}},
				{Name: "Echo", Input: &pb.StringValue{}, Output: &pb.StringValue{}},
				{Name: "GetValue", Input: &pb.Int32Value{}, Output: &pb.Int32Value{}},
				{Name: "Stream", Input: &pb.StringValue{}, Output: &pb.StringValue{}},
			},
		},
	})
	if err != nil {
		panic(err)
	}
}

// TestMethod, EchoMethod, GetValueMethod, and StreamMethod are the methods supported by the mock package.
const (
	TestMethod     = "mock.MockService.Test"
	EchoMethod     = "mock.MockService.Echo"
	GetValueMethod = "mock.MockService.GetValue"
	StreamMethod   = "mock.MockService.Stream"
)

// service represents a service to be registered.
type service struct {
	Name    string // Full package and service name, e.g., "mock.MockService"
	Methods []method
}

// method represents a method in a service.
type method struct {
	Name   string
	Input  proto.Message
	Output proto.Message
}

// registerServices registers the given services in the global registry.
// It is safe to call multiple times, but services with the same package name
// must be registered in the same call or be identical to previous registrations.
// Returns an error if registration fails.
func registerServices(services []service) error {
	// Group by package
	packages := make(map[string][]*descriptorpb.ServiceDescriptorProto)

	for _, s := range services {
		pkgName, svcName, found := strings.Cut(s.Name, ".")
		if !found {
			return fmt.Errorf("service name %q must contain a package", s.Name)
		}

		svcDesc := &descriptorpb.ServiceDescriptorProto{
			Name: new(svcName),
		}

		for _, m := range s.Methods {
			inDesc := m.Input.ProtoReflect().Descriptor()
			outDesc := m.Output.ProtoReflect().Descriptor()
			inName := string(inDesc.FullName())
			outName := string(outDesc.FullName())

			svcDesc.Method = append(svcDesc.Method, &descriptorpb.MethodDescriptorProto{
				Name:       new(m.Name),
				InputType:  new("." + inName),
				OutputType: new("." + outName),
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
			Name:    new(fmt.Sprintf("mock/%s.proto", pkg)),
			Package: new(pkg),
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
			return fmt.Errorf("failed to create file descriptor for %s: %w", pkg, err)
		}

		if err := protoregistry.GlobalFiles.RegisterFile(fileDesc); err != nil {
			return fmt.Errorf("failed to register file %s: %w", pkg, err)
		}
	}
	return nil
}
