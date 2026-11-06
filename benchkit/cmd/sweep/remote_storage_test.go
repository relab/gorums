package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRemoteStorageNamespaceCommand runs the namespace command with sh in a
// temporary root. It must create a private namespace, make an existing one
// private, and refuse a namespace that is a symlink or not a directory, since
// sweep runs cached executables from it. A namespace owned by another user is
// refused by test -O, which a test cannot set up without root.
func TestRemoteStorageNamespaceCommand(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	tests := []struct {
		name    string
		setup   func(t *testing.T, root, namespace string)
		wantErr bool
	}{
		{"Fresh", func(*testing.T, string, string) {}, false},
		{"ExistingOpen", func(t *testing.T, _, namespace string) {
			if err := os.Mkdir(namespace, 0o777); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(namespace, 0o777); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"Symlink", func(t *testing.T, root, namespace string) {
			target := filepath.Join(root, "elsewhere")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, namespace); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"File", func(t *testing.T, _, namespace string) {
			if err := os.WriteFile(namespace, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			namespace := filepath.Join(root, "sweep-ci")
			tt.setup(t, root, namespace)
			out, err := exec.Command("sh", "-c", namespaceCommand(root, namespace)).CombinedOutput()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("namespaceCommand succeeded, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("namespaceCommand: %v\n%s", err, out)
			}
			info, err := os.Lstat(namespace)
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() || info.Mode().Perm() != 0o700 {
				t.Errorf("namespace mode = %v, want a directory with mode 0700", info.Mode())
			}
		})
	}
}
