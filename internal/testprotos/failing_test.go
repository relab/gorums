package testprotos_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/gentest"
)

// failingProto is the frame of each failing input; the test case supplies the
// service and the extra declarations.
const failingProto = `
edition = "2023";

package internal.testprotos.failing;
option go_package = "github.com/relab/gorums/internal/testprotos/failing";

import "gorums.proto";

message Request {
	string value = 1;
}
message Response {
	string value = 1;
}
`

// otherProto is the frame of a second input file in the Go package of
// failingProto; the test case supplies the service.
const otherProto = `
edition = "2023";

package internal.testprotos.failing;
option go_package = "github.com/relab/gorums/internal/testprotos/failing";

import "gorums.proto";
import "failing.proto";
`

// TestFailingProtoFiles verifies that the generator rejects each proto file
// that it cannot generate as written, and reports why.
func TestFailingProtoFiles(t *testing.T) {
	tests := []struct {
		name  string
		decl  string
		other string // declarations of other.proto, if not empty
		// importOnly generates only other.proto; failing.proto is then
		// only an import of the run, as in a separate protoc run.
		importOnly bool
		want       string
	}{
		{
			name: "ReservedMessage",
			decl: `
message Config {
	string value = 1;
}
service Reserved {
	rpc Read(Config) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "contains message Config, which is a reserved Gorums identifier",
		},
		{
			name: "GeneratedClientMessage",
			decl: `
message ReservedClient {
	string value = 1;
}
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains message ReservedClient, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "GeneratedConstructorEnum",
			decl: `
enum NewReservedClient {
	NEW_RESERVED_CLIENT_UNSPECIFIED = 0;
}
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains enum NewReservedClient, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "GeneratedNodeClientMessage",
			decl: `
message ReservedNodeClient {
	string value = 1;
}
service Reserved {
	rpc Read(Request) returns (Response) {}
}`,
			want: "failing.proto: contains message ReservedNodeClient, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "GeneratedNodeConstructorEnum",
			decl: `
enum NewReservedNodeClient {
	NEW_RESERVED_NODE_CLIENT_UNSPECIFIED = 0;
}
service Reserved {
	rpc Write(Request) returns (Response) {
		option (gorums.unicast) = true;
	}
}`,
			want: "failing.proto: contains enum NewReservedNodeClient, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "GeneratedNodeClientWithConfigClient",
			decl: `
message ReservedNodeClient {
	string value = 1;
}
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
	rpc Write(Request) returns (Response) {
		option (gorums.unicast) = true;
	}
}`,
			want: "failing.proto: contains message ReservedNodeClient, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "NestedGeneratedClientMessage",
			decl: `
message Outer {
	message InnerClient {
		string value = 1;
	}
}
service Outer_Inner {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains message Outer_InnerClient, which is the Gorums identifier generated for service Outer_Inner (failing.proto)",
		},
		{
			name: "NestedGeneratedConstructorEnum",
			decl: `
message NewOuter {
	message Middle {
		enum InnerClient {
			INNER_CLIENT_UNSPECIFIED = 0;
		}
	}
}
service Outer_Middle_Inner {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains enum NewOuter_Middle_InnerClient, which is the Gorums identifier generated for service Outer_Middle_Inner (failing.proto)",
		},
		{
			name: "GeneratedClientEnumValue",
			decl: `
enum Outer {
	InnerClient = 0;
}
service Outer_Inner {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains enum value Outer_InnerClient, which is the Gorums identifier generated for service Outer_Inner (failing.proto)",
		},
		{
			name: "NestedGeneratedClientEnumValue",
			decl: `
message Outer {
	enum Kind {
		InnerClient = 0;
	}
}
service Outer_Inner {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains enum value Outer_InnerClient, which is the Gorums identifier generated for service Outer_Inner (failing.proto)",
		},
		{
			name: "GeneratedClientOneofWrapper",
			decl: `
message Outer {
	oneof kind {
		string inner_client = 1;
	}
}
service Outer_Inner {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "contains oneof wrapper Outer_InnerClient, which is the Gorums identifier generated for service Outer_Inner (failing.proto)",
		},
		{
			name: "NestedGeneratedClientOneofWrapper",
			decl: `
message Outer {
	message Middle {
		oneof kind {
			string inner_client = 1;
		}
	}
}
service Outer_Middle_Inner {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "contains oneof wrapper Outer_Middle_InnerClient, which is the Gorums identifier generated for service Outer_Middle_Inner (failing.proto)",
		},
		{
			name: "ReservedEnum",
			decl: `
enum Node {
	NODE_UNSPECIFIED = 0;
}
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "contains enum Node, which is a reserved Gorums identifier",
		},
		{
			name: "SecondServiceServerStreamWithoutQuorumCall",
			decl: `
service Alpha {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
service Beta {
	rpc Watch(Request) returns (stream Response) {}
}`,
			want: "Beta.Watch: option 'gorums.quorumcall' is required for server-client stream methods",
		},
		{
			name: "ServicesGenerateSameClientIdent",
			decl: `
service NewFoo {
	rpc Read(Request) returns (Response) {}
}
service Foo {
	rpc Read(Request) returns (Response) {}
}`,
			want: "failing.proto: services NewFoo and Foo both generate the Gorums identifier NewFooNodeClient",
		},
		{
			name: "ServicesGenerateSameNodeClientIdent",
			decl: `
service FooNode {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
service Foo {
	rpc Read(Request) returns (Response) {}
}`,
			want: "failing.proto: services FooNode and Foo both generate the Gorums identifier FooNodeClient",
		},
		{
			name: "ServicesGenerateSameServerIdent",
			decl: `
service Foo {
	rpc Read(Request) returns (Response) {}
}
service RegisterFoo {
	rpc Read(Request) returns (Response) {}
}`,
			want: "failing.proto: services Foo and RegisterFoo both generate the Gorums identifier RegisterFooServer",
		},
		{
			name: "PackageServicesGenerateSameClientIdent",
			decl: `
service Foo {
	rpc Read(Request) returns (Response) {}
}`,
			other: `
service NewFoo {
	rpc Read(Request) returns (Response) {}
}`,
			want: "failing.proto: services Foo and NewFoo (other.proto) both generate the Gorums identifier NewFooNodeClient",
		},
		{
			name: "ImportedServiceGeneratesSameClientIdent",
			decl: `
service Foo {
	rpc Read(Request) returns (Response) {}
}`,
			other: `
service NewFoo {
	rpc Read(Request) returns (Response) {}
}`,
			importOnly: true,
			want:       "failing.proto: services Foo and NewFoo (other.proto) both generate the Gorums identifier NewFooNodeClient",
		},
		{
			name: "MessageNamedAfterImportedAlias",
			decl: `
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			other: `
message CorrectableResponse {
	string value = 1;
}
service Other {
	rpc Read(Request) returns (Response) {}
}`,
			importOnly: true,
			want:       "other.proto: contains message CorrectableResponse, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "MessageNamedAfterPackageService",
			decl: `
message OtherNodeClient {
	string value = 1;
}
service Reserved {
	rpc Read(Request) returns (Response) {}
}`,
			other: `
service Other {
	rpc Read(Request) returns (Response) {}
}`,
			want: "failing.proto: contains message OtherNodeClient, which is the Gorums identifier generated for service Other (other.proto)",
		},
		{
			name: "PackageMessageNamedAfterService",
			decl: `
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			other: `
message ReservedClient {
	string value = 1;
}`,
			want: "other.proto: contains message ReservedClient, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "GeneratedServerRegisterMessage",
			decl: `
message RegisterReservedServer {
	string value = 1;
}
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains message RegisterReservedServer, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "GeneratedCorrectableMessage",
			decl: `
message CorrectableResponse {
	string value = 1;
}
service Reserved {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains message CorrectableResponse, which is the Gorums identifier generated for service Reserved (failing.proto)",
		},
		{
			name: "PackageEnumNamedAfterCorrectable",
			decl: `
enum CorrectableResponse {
	CORRECTABLE_RESPONSE_UNSPECIFIED = 0;
}
service Reserved {
	rpc Read(Request) returns (Response) {}
}`,
			other: `
service Other {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: contains enum CorrectableResponse, which is the Gorums identifier generated for service Other (other.proto)",
		},
		{
			name: "TypeAliasAndClientIdent",
			decl: `
message FooClient {
	string value = 1;
}
service CorrectableFoo {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
service Bar {
	rpc Read(Request) returns (FooClient) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: services CorrectableFoo and Bar both generate the Gorums identifier CorrectableFooClient",
		},
		{
			name: "TypeAliasAndOwnClientIdent",
			decl: `
message FooClient {
	string value = 1;
}
service CorrectableFoo {
	rpc Read(Request) returns (FooClient) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "failing.proto: service CorrectableFoo generates the Gorums identifier CorrectableFooClient twice",
		},
		{
			name: "ClientStreamWithoutMulticast",
			decl: `
service Stream {
	rpc Send(stream Request) returns (Response) {}
}`,
			want: "Stream.Send: option 'gorums.multicast' is required for client-server stream methods",
		},
		{
			name: "ServerStreamWithoutQuorumCall",
			decl: `
service Stream {
	rpc Watch(Request) returns (stream Response) {}
}`,
			want: "Stream.Watch: option 'gorums.quorumcall' is required for server-client stream methods",
		},
		{
			name: "QuorumCallAndMulticast",
			decl: `
service Conflict {
	rpc Write(Request) returns (Response) {
		option (gorums.quorumcall) = true;
		option (gorums.multicast) = true;
	}
}`,
			want: "Conflict.Write: options 'gorums.quorumcall' and 'gorums.multicast' cannot be combined",
		},
		{
			name: "MulticastAndUnicast",
			decl: `
service Conflict {
	rpc Send(Request) returns (Response) {
		option (gorums.multicast) = true;
		option (gorums.unicast) = true;
	}
}`,
			want: "Conflict.Send: options 'gorums.multicast' and 'gorums.unicast' cannot be combined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := map[string]string{"failing.proto": failingProto + tt.decl}
			protos := []string{"failing.proto"}
			if tt.other != "" {
				sources["other.proto"] = otherProto + tt.other
				protos = append(protos, "other.proto")
			}
			if tt.importOnly {
				protos = []string{"other.proto"}
			}
			dir := gentest.Inline(t, sources)
			files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, protos...)
			if err == nil {
				t.Fatalf("Generate succeeded, want an error containing %q; generated %d files", tt.want, len(files))
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Generate error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestFailingProtoFilesAliasResponseTypes verifies that the generator rejects
// a Go package whose quorum calls return two different response types with the
// same Go name, since both types map to one Correctable alias name.
func TestFailingProtoFilesAliasResponseTypes(t *testing.T) {
	responseProto := func(pkg string) string {
		return `
edition = "2023";

package internal.testprotos.` + pkg + `;
option go_package = "github.com/relab/gorums/internal/testprotos/` + pkg + `";

message Request {
	string value = 1;
}
message Response {
	string value = 1;
}
`
	}
	serviceProto := func(decl string) string {
		return `
edition = "2023";

package internal.testprotos.aliases;
option go_package = "github.com/relab/gorums/internal/testprotos/aliases";

import "gorums.proto";
import "left.proto";
import "right.proto";
` + decl
	}
	alpha := serviceProto(`
service Alpha {
	rpc Read(internal.testprotos.left.Request) returns (internal.testprotos.left.Response) {
		option (gorums.quorumcall) = true;
	}
}`)
	beta := serviceProto(`
service Beta {
	rpc Read(internal.testprotos.right.Request) returns (internal.testprotos.right.Response) {
		option (gorums.quorumcall) = true;
	}
}`)
	gamma := serviceProto(`
service Gamma {
	rpc ReadLeft(internal.testprotos.left.Request) returns (internal.testprotos.left.Response) {
		option (gorums.quorumcall) = true;
	}
	rpc ReadRight(internal.testprotos.right.Request) returns (internal.testprotos.right.Response) {
		option (gorums.quorumcall) = true;
	}
}`)
	const (
		left  = "github.com/relab/gorums/internal/testprotos/left.Response"
		right = "github.com/relab/gorums/internal/testprotos/right.Response"
	)
	tests := []struct {
		name    string
		sources map[string]string
		protos  []string
		want    string
	}{
		{
			name:    "AlphaFirst",
			sources: map[string]string{"alpha.proto": alpha, "beta.proto": beta},
			protos:  []string{"alpha.proto", "beta.proto"},
			want:    "alpha.proto: Alpha.Read returns " + left + " and Beta.Read (beta.proto) returns " + right + "; both map to the Gorums alias CorrectableResponse",
		},
		{
			name:    "BetaFirst",
			sources: map[string]string{"alpha.proto": alpha, "beta.proto": beta},
			protos:  []string{"beta.proto", "alpha.proto"},
			want:    "beta.proto: Beta.Read returns " + right + " and Alpha.Read (alpha.proto) returns " + left + "; both map to the Gorums alias CorrectableResponse",
		},
		{
			name:    "OneService",
			sources: map[string]string{"gamma.proto": gamma},
			protos:  []string{"gamma.proto"},
			want:    "gamma.proto: Gamma.ReadLeft returns " + left + " and Gamma.ReadRight returns " + right + "; both map to the Gorums alias CorrectableResponse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := map[string]string{"left.proto": responseProto("left"), "right.proto": responseProto("right")}
			maps.Copy(sources, tt.sources)
			dir := gentest.Inline(t, sources)
			files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, tt.protos...)
			if err == nil {
				t.Fatalf("Generate succeeded, want an error containing %q; generated %d files", tt.want, len(files))
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Generate error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestFailingProtoFilesPackageFile verifies that the generator rejects a Go
// package with a proto file named gorums.proto in the output directory of the
// package's gorums.pb.go file, since protoc-gen-go writes gorums.pb.go for it.
func TestFailingProtoFilesPackageFile(t *testing.T) {
	const frame = `
edition = "2023";

package internal.testprotos.pkgfile;
option go_package = "github.com/relab/gorums/internal/testprotos/pkgfile";

import "gorums.proto";
`
	const messages = `
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`
	const service = `
service Storage {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`
	const want = "pb/gorums.proto: protoc-gen-go writes gorums.pb.go, which Gorums uses for the package aliases; rename the proto file"
	tests := []struct {
		name    string
		sources map[string]string
		protos  []string
	}{
		{
			name: "ImportedFile",
			sources: map[string]string{
				"pb/gorums.proto":  frame + messages,
				"pb/service.proto": frame + `import "pb/gorums.proto";` + service,
			},
			protos: []string{"pb/service.proto"},
		},
		{
			name:    "GorumsFile",
			sources: map[string]string{"pb/gorums.proto": frame + messages + service},
			protos:  []string{"pb/gorums.proto"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := gentest.Inline(t, tt.sources)
			files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, tt.protos...)
			if err == nil {
				t.Fatalf("Generate succeeded, want an error containing %q; generated %d files", want, len(files))
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Generate error = %q, want it to contain %q", err, want)
			}
		})
	}
}
