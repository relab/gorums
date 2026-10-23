// Package gentest runs the protoc-gen-go and protoc-gen-gorums generators of
// this repository in process, for tests of the generated code.
//
// Only parsing uses the protoc compiler. The generators run from source, so a
// test never depends on a protoc-gen-gorums binary installed on PATH.
// The helpers write nothing into the repository tree, except [Golden] when
// the -update flag is set.
package gentest

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/gengorums"
	"golang.org/x/tools/go/packages"
	gengo "google.golang.org/protobuf/cmd/protoc-gen-go/internal_gengo"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// modulePath is the Go import path of the repository root.
const modulePath = "github.com/relab/gorums"

var update = flag.Bool("update", false, "rewrite golden files with the generated output")

// Options configures [Generate].
type Options struct {
	// ImportPaths lists protoc import paths searched before the repository
	// root, which is always searched so that gorums.proto resolves.
	ImportPaths []string
	// Param is the plugin parameter passed to both generators, such as
	// "paths=source_relative". An empty Param writes output by import path.
	Param string
	// Dev generates one file per call type, as protoc-gen-gorums does with
	// the dev=true parameter.
	Dev bool
}

// Generate parses the proto files with protoc and runs protoc-gen-go and
// protoc-gen-gorums on them. Each file name must be relative to one of the
// import paths. Generate returns the generated files keyed by output path.
// The error is the one a generator reports for the input; Generate fails the
// test if protoc cannot parse the input.
func Generate(t testing.TB, opts Options, files ...string) (map[string]string, error) {
	t.Helper()
	req := request(t, opts.ImportPaths, files)
	goParam := "default_api_level=API_OPAQUE"
	if opts.Param != "" {
		goParam = opts.Param + "," + goParam
	}
	out := make(map[string]string)
	err := run(req, goParam, out, func(gen *protogen.Plugin, f *protogen.File) error {
		gengo.GenerateFile(gen, f)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("protoc-gen-go: %w", err)
	}
	generate := gengorums.GenerateFile
	if opts.Dev {
		generate = gengorums.GenerateDevFiles
	}
	if err := run(req, opts.Param, out, generate); err != nil {
		return nil, err
	}
	return out, nil
}

// request parses files with protoc and returns the plugin request that protoc
// would send to a plugin.
func request(t testing.TB, importPaths, files []string) *pluginpb.CodeGeneratorRequest {
	t.Helper()
	set := filepath.Join(t.TempDir(), "descriptors.pb")
	args := []string{
		"-I" + strings.Join(slices.Concat(importPaths, []string{RootDir()}), string(os.PathListSeparator)),
		"--descriptor_set_out=" + set,
		"--include_imports",
		"--include_source_info",
	}
	cmd := exec.Command("protoc", append(args, files...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("protoc: %v\n%s", err, out)
	}
	data, err := os.ReadFile(set)
	if err != nil {
		t.Fatal(err)
	}
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &fds); err != nil {
		t.Fatal(err)
	}
	return &pluginpb.CodeGeneratorRequest{
		FileToGenerate: files,
		ProtoFile:      fds.GetFile(),
	}
}

// run runs one generator on req with the given parameter, as protoc runs one
// plugin, and adds the generated files to out.
func run(req *pluginpb.CodeGeneratorRequest, param string, out map[string]string, generate func(*protogen.Plugin, *protogen.File) error) error {
	req = proto.CloneOf(req)
	req.Parameter = new(param)
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		return err
	}
	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}
		if err := generate(gen, f); err != nil {
			return err
		}
	}
	resp := gen.Response()
	if resp.Error != nil {
		return errors.New(resp.GetError())
	}
	for _, f := range resp.GetFile() {
		out[f.GetName()] = f.GetContent()
	}
	return nil
}

// Inline writes each proto source to a new temporary directory, keyed by file
// name, and returns the directory. Pass it in [Options.ImportPaths].
func Inline(t testing.TB, sources map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range sources {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Compile type-checks the generated files as packages of this module, without
// writing them to disk. Each file name is an output path by import path, such
// as the names [Generate] returns with an empty [Options.Param]. The
// directories need not exist. Compile fails the test on any error.
func Compile(t testing.TB, files map[string]string) {
	t.Helper()
	root := RootDir()
	overlay := make(map[string][]byte)
	dirs := make(map[string]bool)
	for name, content := range files {
		rel, ok := strings.CutPrefix(name, modulePath+"/")
		if !ok {
			t.Fatalf("Compile: %s is not in module %s", name, modulePath)
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		overlay[path] = []byte(content)
		dirs["./"+filepath.ToSlash(filepath.Dir(rel))] = true
	}
	cfg := &packages.Config{
		Mode:    packages.NeedName | packages.NeedTypes,
		Dir:     root,
		Overlay: overlay,
	}
	pkgs, err := packages.Load(cfg, slices.Collect(maps.Keys(dirs))...)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			t.Errorf("Compile: %v", e)
		}
	})
}

// versionLine matches a tool version line in the header of a generated file.
var versionLine = regexp.MustCompile(`(?m)^(// \t(?:protoc-gen-gorums|protoc-gen-go|protoc) +).+$`)

// Normalize replaces the tool versions in the header of a generated file with
// VERSION, so that comparisons do not depend on the installed protoc or the
// release of a generator.
func Normalize(content string) string {
	return versionLine.ReplaceAllString(content, "${1}VERSION")
}

// Golden compares got, after [Normalize], with the golden file at path, and
// reports a diff if they differ. With the -update flag, Golden writes got to
// path instead.
func Golden(t testing.TB, path, got string) {
	t.Helper()
	got = Normalize(got)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if diff := cmp.Diff(strings.Split(string(want), "\n"), strings.Split(got, "\n")); diff != "" {
		t.Errorf("%s differs (-want +got); run with -update if the change is intended:\n%s", path, diff)
	}
}

// RootDir returns the absolute path of the repository root.
func RootDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}
