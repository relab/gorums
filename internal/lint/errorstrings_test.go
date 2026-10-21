package lint_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// bannedPrefixes start an error string that describes the failure instead of
// the attempted operation, for example "failed to dial node" instead of
// "dialing node".
var bannedPrefixes = []string{"failed ", "could not ", "unable to ", "error "}

// TestErrorStringsDescribeOperation verifies that no errors.New or fmt.Errorf
// call in any module of the repository starts with a failure prefix.
// A leading "gorums: " is ignored.
func TestErrorStringsDescribeOperation(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", ".scratch", "node_modules", "tmp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".pb.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 || !isErrorConstructor(call.Fun) {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			msg, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			msg = strings.TrimPrefix(msg, "gorums: ")
			for _, p := range bannedPrefixes {
				if strings.HasPrefix(msg, p) {
					t.Errorf("%s: error string %q starts with %q; describe the attempted operation instead",
						fset.Position(lit.Pos()), msg, strings.TrimSpace(p))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// isErrorConstructor reports whether fn is errors.New or fmt.Errorf.
func isErrorConstructor(fn ast.Expr) bool {
	sel, ok := fn.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "errors" && sel.Sel.Name == "New" || pkg.Name == "fmt" && sel.Sel.Name == "Errorf"
}
