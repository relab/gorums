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

package gorums.testprotos.failing;
option go_package = "github.com/relab/gorums/internal/testprotos/failing";

import "gorums.proto";

message Request {
	string value = 1;
}
message Response {
	string value = 1;
}
`

// TestFailingProtoFiles verifies that the generator rejects each proto file
// that it cannot generate as written, and reports why.
func TestFailingProtoFiles(t *testing.T) {
	tests := []struct {
		name string
		decl string
		want string
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
			name: "ReservedRPCMethod",
			decl: `
service Reserved {
	rpc Config(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "contains RPC method Config, which is a reserved Gorums identifier",
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
			name: "MultipleServices",
			decl: `
service Alpha {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
service Beta {
	rpc Write(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			want: "Gorums does not support multiple services in the same proto file",
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
			dir := gentest.Inline(t, map[string]string{"failing.proto": failingProto + tt.decl})
			files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, "failing.proto")
			if err == nil {
				t.Fatalf("Generate succeeded, want an error containing %q; generated %d files", tt.want, len(files))
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Generate error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}
