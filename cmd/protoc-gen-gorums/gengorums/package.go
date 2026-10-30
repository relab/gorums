package gengorums

import (
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// goPackage is the view of one Go package in a protoc run. The generator
// computes it once per run, so that the rules that tie the files of a package
// together are in one place.
type goPackage struct {
	// files holds every file of the package in the order of gen.Files,
	// including the files that are only imports of this run.
	files []*protogen.File
	// gorumsFiles holds the files of the package that have Gorums methods.
	// The run that generates such a file emits its Gorums code into this Go
	// package, even if the file is only an import of this run.
	gorumsFiles []*protogen.File
	// owner is the first file of gorumsFiles that this run generates. The
	// package file, which holds the static code, is generated with the owner.
	// It is nil if this run generates no Gorums code for the package.
	owner *protogen.File
	// idents maps the package-level identifiers that Gorums generates for the
	// services of gorumsFiles to the service that generates each one.
	idents map[string]*protogen.Service
	// err is the error for two services that generate the same identifier.
	err error
}

// newGoPackages returns the Go packages of the run, keyed by import path.
func newGoPackages(gen *protogen.Plugin) map[protogen.GoImportPath]*goPackage {
	pkgs := make(map[protogen.GoImportPath]*goPackage)
	for _, f := range gen.Files {
		pkg := pkgs[f.GoImportPath]
		if pkg == nil {
			pkg = &goPackage{idents: make(map[string]*protogen.Service)}
			pkgs[f.GoImportPath] = pkg
		}
		pkg.files = append(pkg.files, f)
		if !hasGorumsMethods(f.Services) {
			continue
		}
		pkg.gorumsFiles = append(pkg.gorumsFiles, f)
		if pkg.owner == nil && f.Generate {
			pkg.owner = f
		}
	}
	for _, pkg := range pkgs {
		pkg.collectIdents()
	}
	return pkgs
}

// collectIdents fills idents with the identifiers that Gorums generates for
// the services of gorumsFiles. It sets err if two services generate the same
// identifier.
func (p *goPackage) collectIdents() {
	for _, f := range p.gorumsFiles {
		for _, service := range f.Services {
			for _, name := range serviceIdents(service) {
				if first, ok := p.idents[name]; ok {
					p.err = identCollisionError(first, service, name)
					return
				}
				p.idents[name] = service
			}
		}
	}
}

// serviceIdents returns the package-level identifiers that Gorums generates
// for the given service. No two of them are equal.
func serviceIdents(service *protogen.Service) []string {
	var idents []string
	for _, kind := range clientKinds {
		if len(kind.methods(service)) > 0 {
			idents = append(idents, clientNamesOf(service, kind).idents()...)
		}
	}
	return append(idents, serverNamesOf(service).idents()...)
}

// identCollisionError returns the error for two services, first and second,
// that both generate the identifier name. The error names the file of first,
// and the file of second if it differs.
func identCollisionError(first, second *protogen.Service, name string) error {
	firstPath, secondPath := first.Desc.ParentFile().Path(), second.Desc.ParentFile().Path()
	secondName := string(second.Desc.Name())
	if secondPath != firstPath {
		secondName += " (" + secondPath + ")"
	}
	return fmt.Errorf("%s: services %s and %s both generate the Gorums identifier %s",
		firstPath, first.Desc.Name(), secondName, name)
}

// checkReservedIdents returns an error if a Go declaration of the package
// collides with an identifier that Gorums declares in that package. Two
// services of the package must not generate the same identifier. A message,
// enum, or enum value of any file in the package, nested or not, must not take
// the name of a Gorums type alias or of an identifier generated for one of
// these services. For example, service NewS generates the interface NewSClient,
// and service S generates the constructor NewSClient.
//
// The check treats every service of the Gorums files as a Gorums service,
// because a method without a Gorums option is a remote call. A package may
// also hold a plain gRPC service in a file that is compiled without the Gorums
// plugin. The check then reserves the node client names of that service, such
// as SNodeClient, although protoc-gen-go-grpc generates SClient, NewSClient,
// and sClient instead. A clash with those names shows when the package
// compiles.
func (p *goPackage) checkReservedIdents() error {
	if p.err != nil {
		return p.err
	}
	for _, f := range p.files {
		check := func(kind, name string) error {
			switch service := p.idents[name]; {
			case slices.Contains(reservedIdents, name):
				return fmt.Errorf("%s: contains %s %s, which is a reserved Gorums identifier", f.Desc.Path(), kind, name)
			case service != nil:
				return fmt.Errorf("%s: contains %s %s, which is the Gorums identifier generated for service %s (%s)",
					f.Desc.Path(), kind, name, service.Desc.Name(), service.Desc.ParentFile().Path())
			}
			return nil
		}
		if err := checkTypes(f.Messages, f.Enums, check); err != nil {
			return err
		}
	}
	return nil
}

// checkTypes calls check with the Go name of each message, enum, enum value,
// and oneof wrapper, including nested ones, since protoc-gen-go declares every
// nested type, every enum value, and every oneof wrapper at package level. The
// opaque API declares a oneof wrapper with a lowercase first letter, and the
// Go API level is not known here, so checkTypes passes both spellings.
func checkTypes(msgs []*protogen.Message, enums []*protogen.Enum, check func(kind, name string) error) error {
	for _, enum := range enums {
		if err := check("enum", enum.GoIdent.GoName); err != nil {
			return err
		}
		for _, value := range enum.Values {
			if err := check("enum value", value.GoIdent.GoName); err != nil {
				return err
			}
		}
	}
	for _, msg := range msgs {
		if err := check("message", msg.GoIdent.GoName); err != nil {
			return err
		}
		for _, oneof := range msg.Oneofs {
			if oneof.Desc.IsSynthetic() {
				continue
			}
			for _, field := range oneof.Fields {
				name := field.GoIdent.GoName
				for _, wrapper := range []string{name, strings.ToLower(name[:1]) + name[1:]} {
					if err := check("oneof wrapper", wrapper); err != nil {
						return err
					}
				}
			}
		}
		if err := checkTypes(msg.Messages, msg.Enums, check); err != nil {
			return err
		}
	}
	return nil
}
