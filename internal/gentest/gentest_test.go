package gentest_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/gentest"
)

const echoProto = `
edition = "2023";

package gentest.echo;
option go_package = "github.com/relab/gorums/internal/gentest/testdata/echo";

import "gorums.proto";

service Echo {
	rpc Echo(Msg) returns (Msg) {
		option (gorums.quorumcall) = true;
	}
}

message Msg {
	string value = 1;
}
`

func TestGentestGenerate(t *testing.T) {
	dir := gentest.Inline(t, map[string]string{"echo.proto": echoProto})
	files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, "echo.proto")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "github.com/relab/gorums/internal/gentest/testdata/echo/echo"
	for _, name := range []string{prefix + ".pb.go", prefix + "_gorums.pb.go"} {
		if _, ok := files[name]; !ok {
			t.Errorf("Generate did not return %s; got %v", name, slices.Sorted(maps.Keys(files)))
		}
	}
	if got := files[prefix+"_gorums.pb.go"]; !strings.Contains(got, "func (c echoClient) Echo(") {
		t.Errorf("generated Gorums code has no echoClient.Echo method:\n%s", got)
	}
	gentest.Compile(t, files)
}

func TestGentestGenerateNoGorums(t *testing.T) {
	dir := gentest.Inline(t, map[string]string{"echo.proto": echoProto})
	files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}, NoGorums: true}, "echo.proto")
	if err != nil {
		t.Fatal(err)
	}
	const name = "github.com/relab/gorums/internal/gentest/testdata/echo/echo.pb.go"
	if got := slices.Sorted(maps.Keys(files)); !slices.Equal(got, []string{name}) {
		t.Errorf("Generate returned %v, want only %s", got, name)
	}
}

func TestGentestGenerateDev(t *testing.T) {
	dir := gentest.Inline(t, map[string]string{"echo.proto": echoProto})
	files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}, Dev: true}, "echo.proto")
	if err != nil {
		t.Fatal(err)
	}
	const name = "github.com/relab/gorums/internal/gentest/testdata/echo/echo_quorumcall_gorums.pb.go"
	if _, ok := files[name]; !ok {
		t.Errorf("Generate did not return %s; got %v", name, slices.Sorted(maps.Keys(files)))
	}
}

func TestGentestGenerateError(t *testing.T) {
	src := strings.Replace(echoProto, "message Msg {", "message EchoClient {", 1)
	src = strings.ReplaceAll(src, "(Msg)", "(EchoClient)")
	dir := gentest.Inline(t, map[string]string{"echo.proto": src})
	_, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, "echo.proto")
	const want = "echo.proto: contains message EchoClient, which is the Gorums identifier generated for service Echo (echo.proto)"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Generate error = %v, want it to contain %q", err, want)
	}
}

func TestGentestNormalize(t *testing.T) {
	in := "// versions:\n// \tprotoc-gen-gorums v0.11.0-devel\n// \tprotoc            v7.35.1\n// \tprotoc-gen-go v1.36.12\n// \tprotoc        (unknown)\n// source: a.proto\n"
	want := "// versions:\n// \tprotoc-gen-gorums VERSION\n// \tprotoc            VERSION\n// \tprotoc-gen-go VERSION\n// \tprotoc        VERSION\n// source: a.proto\n"
	if got := gentest.Normalize(in); got != want {
		t.Errorf("Normalize() =\n%s\nwant:\n%s", got, want)
	}
}

func TestGentestRootDir(t *testing.T) {
	root := gentest.RootDir(t)
	if !filepath.IsAbs(root) {
		t.Errorf("RootDir() = %q, want an absolute path", root)
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "module github.com/relab/gorums\n") {
		t.Errorf("RootDir() = %q, whose go.mod is not the gorums module", root)
	}
}
