package testprotos_test

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/gentest"
)

// fixtureDir is the path of this package relative to the repository root.
const fixtureDir = "internal/testprotos"

// fixtures returns the proto files under this package, relative to the
// repository root. Files under testdata are not fixtures.
func fixtures(t *testing.T) []string {
	t.Helper()
	var protos []string
	err := fs.WalkDir(os.DirFS("."), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return fs.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".proto") {
			protos = append(protos, path.Join(fixtureDir, p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(protos) == 0 {
		t.Fatal("no proto fixtures found")
	}
	return protos
}

// generateFixtures runs the generators on all fixtures in one protoc run,
// so that fixtures may import each other.
func generateFixtures(t *testing.T) map[string]string {
	t.Helper()
	files, err := gentest.Generate(t, gentest.Options{}, fixtures(t)...)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestGenerateProtoFiles checks that the code generated for each fixture
// compiles.
func TestGenerateProtoFiles(t *testing.T) {
	gentest.Compile(t, generateFixtures(t))
}

// TestGoldenProtoFiles compares the Gorums code generated for each fixture
// with its golden file in testdata. Run with -update to rewrite the golden
// files after an intended change to the generator.
func TestGoldenProtoFiles(t *testing.T) {
	files := generateFixtures(t)
	for _, proto := range fixtures(t) {
		rel := strings.TrimPrefix(strings.TrimSuffix(proto, ".proto"), fixtureDir+"/")
		golden := filepath.Join("testdata", filepath.FromSlash(rel)+"_gorums.pb.go.golden")
		t.Run(rel, func(t *testing.T) {
			name := gorumsFile(files, path.Base(rel))
			if name == "" {
				// The fixture has no Gorums methods; there must be no golden file.
				if _, err := os.Stat(golden); err == nil {
					t.Errorf("no Gorums output for %s, but %s exists", proto, golden)
				}
				return
			}
			gentest.Golden(t, golden, files[name])
		})
	}
}

// gorumsFile returns the name of the generated Gorums file whose base name is
// base + "_gorums.pb.go", or "" if there is none.
func gorumsFile(files map[string]string, base string) string {
	for name := range files {
		if path.Base(name) == base+"_gorums.pb.go" {
			return name
		}
	}
	return ""
}

// TestGenerateImportedPackageNames checks that the generated code compiles
// when its messages come from a Go package whose name equals a local name of
// the generated code: the client method receiver c, the client method
// parameters ctx, in, and node, or the server handler parameters ctx and in.
func TestGenerateImportedPackageNames(t *testing.T) {
	for _, pkg := range []string{"c", "ctx", "in", "node"} {
		t.Run(pkg, func(t *testing.T) {
			msgs := fmt.Sprintf(`
edition = "2023";

package gorums.testprotos.shadow.%[1]s;
option go_package = "github.com/relab/gorums/internal/testprotos/shadow/%[1]s";

message Request {
	string value = 1;
}
message Response {
	string value = 1;
}
`, pkg)
			service := fmt.Sprintf(`
edition = "2023";

package gorums.testprotos.shadow.service;
option go_package = "github.com/relab/gorums/internal/testprotos/shadow/service";

import "gorums.proto";
import "%[1]s.proto";

service Shadow {
	rpc QuorumCall(%[2]s.Request) returns (%[2]s.Response) {
		option (gorums.quorumcall) = true;
	}
	rpc QuorumCallStream(%[2]s.Request) returns (stream %[2]s.Response) {
		option (gorums.quorumcall) = true;
	}
	rpc Multicast(%[2]s.Request) returns (%[2]s.Response) {
		option (gorums.multicast) = true;
	}
	rpc Unicast(%[2]s.Request) returns (%[2]s.Response) {
		option (gorums.unicast) = true;
	}
	rpc RemoteCall(%[2]s.Request) returns (%[2]s.Response) {}
}
`, pkg, "gorums.testprotos.shadow."+pkg)
			dir := gentest.Inline(t, map[string]string{pkg + ".proto": msgs, "service.proto": service})
			files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, pkg+".proto", "service.proto")
			if err != nil {
				t.Fatal(err)
			}
			gentest.Compile(t, files)
		})
	}
}
