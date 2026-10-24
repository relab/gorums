package testprotos_test

import (
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
		want  string
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
