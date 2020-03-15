package internalgorums

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"text/template"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func DISABLEDTestNodeImports(t *testing.T) {
	const (
		filename         = "dir/filename.proto"
		protoPackageName = "proto.package"
	)
	req := &pluginpb.CodeGeneratorRequest{
		Parameter: proto.String(""),
		ProtoFile: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String(filename),
				Package: proto.String(protoPackageName),
				Options: &descriptorpb.FileOptions{
					GoPackage: proto.String("dir;foo"),
				},
			},
		},
		FileToGenerate: []string{filename},
	}

	gen, err := protogen.New(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	g := gen.NewGeneratedFile("foo.go", "gorums.io/x/foo")
	g.P("package foo")
	g.P()
	file := gen.Files[0]

	var nodeTemplate = template.Must(template.New("").Funcs(funcMap).Parse(
		nodeServices,
	))
	data := struct {
		GenFile  *protogen.GeneratedFile
		Services []*protogen.Service
	}{
		g,
		file.Services,
	}
	g.P(mustExecute(nodeTemplate, data))
	g.P()
	g.Content()
	got, err := g.Content()
	if err != nil {
		t.Fatalf("g.Content() = %v", err)
	}

	want := `package foo

import (
	context "context"
	fmt "fmt"
	grpc "google.golang.org/grpc"
	log "log"
	sync "sync"
	time "time"
)
`

	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Fatalf("content mismatch (-want +got):\n%s", diff)
	}
}

func DisabledTestNodeStruct(t *testing.T) {
	const (
		filename         = "dir/node.proto"
		protoPackageName = "proto.package"
		protofile        = `
syntax = "proto3";
package node;
option go_package = "gorums/io/node;node";
message Request {}
message Response {}

service test_service {
  rpc unary_call(Request) returns (Response);

  // This RPC streams from the server only.
  rpc downstream_call(Request) returns (stream Response);

  // This RPC streams from the client.
  rpc upstream_call(stream Request) returns (Response);

  // This one streams in both directions.
  rpc bidi_call(stream Request) returns (stream Response);
}
`
	)

	repoRoot := ""
	tmpDir, err := ioutil.TempDir(repoRoot, "tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	//TODO(meling) make special test plugin that only generates code I'm interested in.

	req := &pluginpb.CodeGeneratorRequest{
		Parameter: proto.String(""),
		ProtoFile: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String(filename),
				Package: proto.String(protoPackageName),
				Options: &descriptorpb.FileOptions{
					GoPackage: proto.String("dir;foo"),
				},
			},
		},
		FileToGenerate: []string{filename},
	}

	gen, err := protogen.New(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	g := gen.NewGeneratedFile("node_struct.go", "gorums.io/x/node")
	g.P("package gorums")
	g.P()
	file := gen.Files[0]

	var nodeTemplate = template.Must(template.New("").Funcs(funcMap).Parse(
		nodeServices + nodeConnectStream,
	))
	data := struct {
		GenFile  *protogen.GeneratedFile
		Services []*protogen.Service
	}{
		g,
		file.Services,
	}
	g.P(mustExecute(nodeTemplate, data))
	g.P()
	g.Content()
	got, err := g.Content()
	if err != nil {
		t.Fatalf("g.Content() = %v", err)
	}

	want := `package foo

import (
	context "context"
	fmt "fmt"
	grpc "google.golang.org/grpc"
	log "log"
	sync "sync"
	time "time"
)
`

	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Fatalf("content mismatch (-want +got):\n%s", diff)
	}
}

func protoc(outpath string, args ...string) {
	// @protoc -I../../..:. \
	// cmd/protoc-gen-gorums/tests/quorumcall/qc.proto
	// --go_out=<param1>=<value1>,<param2>=<value2>:<output_directory>
	cmd := exec.Command("protoc", "-I../../..:"+outpath, "--gorums_out="+outpath)
	cmd.Args = append(cmd.Args, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("executing: %v\n%s\n", strings.Join(cmd.Args, " "), out)
	}
}
