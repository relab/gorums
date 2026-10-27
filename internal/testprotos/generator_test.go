package testprotos_test

import (
	"cmp"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
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
	// Each Go package of the fixtures gets one gorums.pb.go file, with the
	// golden file testdata/<package directory>/gorums.pb.go.golden.
	goldens := make(map[string]bool)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if path.Base(name) != packageFile {
			continue
		}
		rel, ok := strings.CutPrefix(path.Dir(name), modulePrefix+fixtureDir+"/")
		if !ok {
			t.Errorf("%s is not under %s", name, fixtureDir)
			continue
		}
		golden := filepath.Join("testdata", filepath.FromSlash(rel), packageFile+".golden")
		goldens[golden] = true
		t.Run(path.Join(rel, packageFile), func(t *testing.T) {
			gentest.Golden(t, golden, files[name])
		})
	}
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == packageFile+".golden" && !goldens[p] {
			t.Errorf("no %s output for %s, but %s exists", packageFile, filepath.Dir(p), p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// packageFile is the name of the file that holds the static aliases of a Go
// package.
const packageFile = "gorums.pb.go"

// modulePrefix is the import path prefix of the packages of this module.
const modulePrefix = "github.com/relab/gorums/"

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

// protoFrame returns the frame of an input file of [TestGeneratePackageFiles]
// in the Go package pkg under this package.
func protoFrame(pkg string) string {
	return `
edition = "2023";

package gorums.testprotos.` + pkg + `;
option go_package = "github.com/relab/gorums/internal/testprotos/` + pkg + `";

import "gorums.proto";
`
}

// aliasDecl matches the Config alias of the alias block that the generator
// emits into the gorums.pb.go file of each Go package.
var aliasDecl = regexp.MustCompile(`Config\s*=\s*gorums\.Config\b`)

// typeAliasDecl matches the declaration of a Correctable alias and captures
// its name.
var typeAliasDecl = regexp.MustCompile(`(?m)^type (Correctable\w+) =`)

// TestGeneratePackageFiles checks that the code generated for several proto
// files of one Go package compiles as one package. Each Go package with
// Gorums methods gets one gorums.pb.go file, which alone declares the static
// aliases (Config, ID, and Node). Each Correctable alias is declared only in
// the Gorums file of the first proto file whose quorum calls produce it, in
// protoc's file order. Imported files come first in that order.
func TestGeneratePackageFiles(t *testing.T) {
	tests := []struct {
		name          string
		sources       map[string]string   // declarations, keyed by proto file name
		packages      map[string]string   // Go package of a proto file, if not pkgfiles
		aliasPackages []string            // Go packages that get a gorums.pb.go file
		typeAliases   map[string][]string // Correctable aliases declared, keyed by proto file
	}{
		{
			name: "OneServicePerFile",
			sources: map[string]string{
				"alpha.proto": `
service Alpha {
	rpc Read(AlphaRequest) returns (AlphaResponse) {
		option (gorums.quorumcall) = true;
	}
}
message AlphaRequest {
	string value = 1;
}
message AlphaResponse {
	string value = 1;
}`,
				"beta.proto": `
service Beta {
	rpc Write(BetaRequest) returns (BetaResponse) {
		option (gorums.quorumcall) = true;
	}
	rpc Get(BetaRequest) returns (BetaResponse) {}
}
message BetaRequest {
	string value = 1;
}
message BetaResponse {
	string value = 1;
}`,
			},
			aliasPackages: []string{"pkgfiles"},
			typeAliases: map[string][]string{
				"alpha.proto": {"CorrectableAlphaResponse"},
				"beta.proto":  {"CorrectableBetaResponse"},
			},
		},
		{
			// Both services return Response; only alpha.proto declares its aliases.
			name: "SharedResponseType",
			sources: map[string]string{
				"alpha.proto": `
service Alpha {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`,
				"beta.proto": `
import "alpha.proto";

service Beta {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			},
			aliasPackages: []string{"pkgfiles"},
			typeAliases: map[string][]string{
				"alpha.proto": {"CorrectableResponse"},
			},
		},
		{
			// The first file has no Gorums methods and gets no Gorums file.
			name: "MessagesOnlyFirstFile",
			sources: map[string]string{
				"a.proto": `
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`,
				"b.proto": `
import "a.proto";

service Beta {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			},
			aliasPackages: []string{"pkgfiles"},
			typeAliases: map[string][]string{
				"b.proto": {"CorrectableResponse"},
			},
		},
		{
			// protoc places the imported b.proto before a.proto. Both files
			// return Response, so b.proto declares its aliases; a.proto
			// declares only the aliases of AlphaResponse.
			name: "ImportedFileFirst",
			sources: map[string]string{
				"a.proto": `
import "b.proto";

service Alpha {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
	rpc Write(Request) returns (AlphaResponse) {
		option (gorums.quorumcall) = true;
	}
}
message AlphaResponse {
	string value = 1;
}`,
				"b.proto": `
service Beta {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`,
			},
			aliasPackages: []string{"pkgfiles"},
			typeAliases: map[string][]string{
				"a.proto": {"CorrectableAlphaResponse"},
				"b.proto": {"CorrectableResponse"},
			},
		},
		{
			// Each Go package declares its own aliases of Response.
			name: "TwoGoPackages",
			sources: map[string]string{
				"alpha.proto": `
service Alpha {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`,
				"beta.proto": `
service Beta {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`,
			},
			packages:      map[string]string{"beta.proto": "pkgother"},
			aliasPackages: []string{"pkgfiles", "pkgother"},
			typeAliases: map[string][]string{
				"alpha.proto": {"CorrectableResponse"},
				"beta.proto":  {"CorrectableResponse"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := make(map[string]string, len(tt.sources))
			for name, decl := range tt.sources {
				pkg := cmp.Or(tt.packages[name], "pkgfiles")
				sources[name] = protoFrame(pkg) + decl
			}
			dir := gentest.Inline(t, sources)
			protos := slices.Sorted(maps.Keys(sources))
			files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, protos...)
			if err != nil {
				t.Fatal(err)
			}
			for _, proto := range protos {
				base := strings.TrimSuffix(proto, ".proto")
				// Only a proto file with a service gets a Gorums file.
				if strings.Contains(tt.sources[proto], "service ") && gorumsFile(files, base) == "" {
					t.Errorf("no Gorums output for %s; generated files: %v", proto, slices.Sorted(maps.Keys(files)))
				}
			}
			compileWithAliases(t, files, tt.aliasPackages)
			checkAliasFiles(t, files, tt.aliasPackages)
			checkTypeAliases(t, files, tt.typeAliases)
		})
	}
}

// TestGeneratePackageFilesSeparateRuns checks that the code generated for the
// proto files of one Go package in separate protoc runs compiles as one
// package. Each run generates one file, and every run with Gorums methods
// writes the same gorums.pb.go file. A run leaves out the Correctable aliases
// that an imported file of the package owns, because the run of the imported
// file declares them. A file whose service is plain gRPC is generated
// without Gorums.
func TestGeneratePackageFilesSeparateRuns(t *testing.T) {
	// imported declares the Correctable alias of Response; importing
	// declares only the Correctable alias of OtherResponse.
	const imported = `
service Imported {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`
	const importing = `
service Importing {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
	rpc Write(Request) returns (OtherResponse) {
		option (gorums.quorumcall) = true;
	}
}
message OtherResponse {
	string value = 1;
}`
	const alpha = `
service Alpha {
	rpc Read(AlphaRequest) returns (AlphaResponse) {
		option (gorums.quorumcall) = true;
	}
}
message AlphaRequest {
	string value = 1;
}
message AlphaResponse {
	string value = 1;
}`
	const beta = `
service Beta {
	rpc Read(BetaRequest) returns (BetaResponse) {
		option (gorums.quorumcall) = true;
	}
}
message BetaRequest {
	string value = 1;
}
message BetaResponse {
	string value = 1;
}`
	// plain has an unannotated method, which Gorums would generate as a
	// remote call; its run generates only protoc-gen-go code.
	const plain = `
service Plain {
	rpc Get(Request) returns (Response) {}
}
message Request {
	string value = 1;
}
message Response {
	string value = 1;
}`
	tests := []struct {
		name        string
		sources     map[string]string   // declarations, keyed by proto file name
		runs        []string            // the proto file of each run, in run order
		noGorums    []string            // proto files whose run has no Gorums
		typeAliases map[string][]string // Correctable aliases declared, keyed by proto file
	}{
		{
			name: "ImportedFileFirst",
			sources: map[string]string{
				"a.proto": imported,
				"b.proto": `import "a.proto";` + importing,
			},
			runs: []string{"a.proto", "b.proto"},
			typeAliases: map[string][]string{
				"a.proto": {"CorrectableResponse"},
				"b.proto": {"CorrectableOtherResponse"},
			},
		},
		{
			name: "ImportingFileFirst",
			sources: map[string]string{
				"a.proto": `import "b.proto";` + importing,
				"b.proto": imported,
			},
			runs: []string{"b.proto", "a.proto"},
			typeAliases: map[string][]string{
				"a.proto": {"CorrectableOtherResponse"},
				"b.proto": {"CorrectableResponse"},
			},
		},
		{
			name:    "NoImport",
			sources: map[string]string{"a.proto": alpha, "b.proto": beta},
			runs:    []string{"a.proto", "b.proto"},
			typeAliases: map[string][]string{
				"a.proto": {"CorrectableAlphaResponse"},
				"b.proto": {"CorrectableBetaResponse"},
			},
		},
		{
			// The imported plain.proto comes first in protoc's file order of
			// the Gorums run, but it has no Gorums file.
			name: "PlainGRPCImport",
			sources: map[string]string{
				"plain.proto": plain,
				"service.proto": `import "plain.proto";
service Storage {
	rpc Read(Request) returns (Response) {
		option (gorums.quorumcall) = true;
	}
}`,
			},
			runs:     []string{"plain.proto", "service.proto"},
			noGorums: []string{"plain.proto"},
			typeAliases: map[string][]string{
				"service.proto": {"CorrectableResponse"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := make(map[string]string, len(tt.sources))
			for name, decl := range tt.sources {
				sources[name] = protoFrame("pkgfiles") + decl
			}
			dir := gentest.Inline(t, sources)
			files := make(map[string]string)
			for _, proto := range tt.runs {
				opts := gentest.Options{ImportPaths: []string{dir}, NoGorums: slices.Contains(tt.noGorums, proto)}
				out, err := gentest.Generate(t, opts, proto)
				if err != nil {
					t.Fatalf("%s: %v", proto, err)
				}
				for name, content := range out {
					prev, ok := files[name]
					switch {
					case !ok:
					case path.Base(name) != packageFile:
						t.Fatalf("%s: two runs generate %s", proto, name)
					case prev != content:
						t.Errorf("%s: the runs generate different %s files:\n%s\n---\n%s", proto, name, prev, content)
					}
				}
				maps.Copy(files, out)
			}
			compileWithAliases(t, files, []string{"pkgfiles"})
			checkAliasFiles(t, files, []string{"pkgfiles"})
			checkTypeAliases(t, files, tt.typeAliases)
		})
	}
}

// checkTypeAliases checks that each generated Gorums file declares exactly
// the Correctable aliases listed for its proto file in want.
func checkTypeAliases(t *testing.T, files map[string]string, want map[string][]string) {
	t.Helper()
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if !strings.HasSuffix(name, "_gorums.pb.go") {
			continue
		}
		var got []string
		for _, m := range typeAliasDecl.FindAllStringSubmatch(files[name], -1) {
			got = append(got, m[1])
		}
		slices.Sort(got)
		proto := strings.TrimSuffix(path.Base(name), "_gorums.pb.go") + ".proto"
		if wantNames := slices.Sorted(slices.Values(want[proto])); !slices.Equal(got, wantNames) {
			t.Errorf("%s declares type aliases %v, want %v", name, got, wantNames)
		}
	}
}

// checkAliasFiles checks that the gorums.pb.go files of the given Go packages
// are the only generated files that declare the static aliases.
func checkAliasFiles(t *testing.T, files map[string]string, pkgs []string) {
	t.Helper()
	var want []string
	for _, pkg := range pkgs {
		want = append(want, pkgDir(pkg)+"/"+packageFile)
	}
	slices.Sort(want)
	var got []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if aliasDecl.MatchString(files[name]) {
			got = append(got, name)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("files with the static aliases: %v, want %v", got, want)
	}
}

// compileWithAliases compiles the generated files together with a file in
// each of the given Go packages that uses the static aliases.
func compileWithAliases(t *testing.T, files map[string]string, pkgs []string) {
	t.Helper()
	all := maps.Clone(files)
	for _, pkg := range pkgs {
		all[pkgDir(pkg)+"/use_aliases.go"] = "package " + pkg + "\n\nvar (\n\t_ Config\n\t_ ID\n\t_ *Node\n)\n"
	}
	gentest.Compile(t, all)
}

// pkgDir returns the output directory, by import path, of the Go package pkg
// under this package.
func pkgDir(pkg string) string {
	return modulePrefix + fixtureDir + "/" + pkg
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

package internal.testprotos.shadow.%[1]s;
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

package internal.testprotos.shadow.service;
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
`, pkg, "internal.testprotos.shadow."+pkg)
			dir := gentest.Inline(t, map[string]string{pkg + ".proto": msgs, "service.proto": service})
			files, err := gentest.Generate(t, gentest.Options{ImportPaths: []string{dir}}, pkg+".proto", "service.proto")
			if err != nil {
				t.Fatal(err)
			}
			gentest.Compile(t, files)
		})
	}
}
