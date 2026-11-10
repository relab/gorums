package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testPub  = "ssh-ed25519 AAAAnew gorums-release-smoke-2026-11-11"
	testPriv = "-----BEGIN OPENSSH PRIVATE KEY-----\nnew\n-----END OPENSSH PRIVATE KEY-----\n"
)

// keygenRunner returns a fake runner whose ssh-keygen writes the test key pair
// to the file that -f names.
func keygenRunner(t *testing.T) *fakeRunner {
	f := newFakeRunner()
	f.answers["gh api user"] = "4242\n"
	f.onExec = func(line string) {
		if !strings.HasPrefix(line, "ssh-keygen ") {
			return
		}
		_, path, ok := strings.Cut(line, " -f ")
		if !ok {
			t.Fatalf("ssh-keygen without -f: %s", line)
		}
		if err := os.WriteFile(path, []byte(testPriv), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+".pub", []byte(testPub+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// stdinOf returns the standard input of the one recorded command that starts
// with prefix.
func stdinOf(t *testing.T, f *fakeRunner, prefix string) string {
	t.Helper()
	var found []string
	for line, in := range f.stdin {
		if strings.HasPrefix(line, prefix) {
			found = append(found, in)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d commands with stdin start with %q, want 1", len(found), prefix)
	}
	return found[0]
}

func defaultClusterKeyOptions() clusterKeyOptions {
	return clusterKeyOptions{jump: "gorums@ssh4.ux.uis.no", cluster: "gorums@bb1"}
}

func TestClusterKeyAdd(t *testing.T) {
	f := keygenRunner(t)
	tl, out := newTestTool(t, f)
	if err := tl.clusterKey(defaultClusterKeyOptions()); err != nil {
		t.Fatal(err)
	}
	assertOrder(t, f.calls,
		"query gh auth status",
		"exec ssh-keygen -q -t ed25519 -N  -C gorums-release-smoke-2026-11-11 -f ",
		"exec ssh gorums@ssh4.ux.uis.no ",
		"exec ssh gorums@bb1 ",
		"query gh api user --jq .id",
		"exec gh api --method PUT repos/{owner}/{repo}/environments/cluster --input -",
		"query gh api repos/{owner}/{repo}/environments/cluster/deployment-branch-policies --jq .branch_policies[] | \"\\(.id) \\(.name)\"",
		"exec gh api --method POST repos/{owner}/{repo}/environments/cluster/deployment-branch-policies -f name=release/v* -f type=branch",
		"exec gh secret set CLUSTER_SSH_KEY --env cluster",
		"exec gh api --method PUT repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval -f approval_policy=all_external_contributors",
	)

	jump := stdinOf(t, f, "ssh gorums@ssh4.ux.uis.no ")
	wantJump := `command="/bin/false",restrict,port-forwarding,permitopen="bb1.ux.uis.no:22" ` + testPub + "\n"
	if jump != wantJump {
		t.Errorf("jump host line = %q, want %q", jump, wantJump)
	}
	cluster := stdinOf(t, f, "ssh gorums@bb1 ")
	wantCluster := `from="152.94.1.60,152.94.162.0/26",restrict,port-forwarding,agent-forwarding,pty ` + testPub + "\n"
	if cluster != wantCluster {
		t.Errorf("cluster line = %q, want %q", cluster, wantCluster)
	}
	if got := stdinOf(t, f, "gh secret set"); got != testPriv {
		t.Errorf("secret = %q, want the private key", got)
	}
	env := stdinOf(t, f, "gh api --method PUT repos/{owner}/{repo}/environments/cluster")
	for _, want := range []string{`"id":4242`, `"type":"User"`, `"custom_branch_policies":true`, `"protected_branches":false`} {
		if !strings.Contains(env, want) {
			t.Errorf("environment request = %s, want it to contain %s", env, want)
		}
	}

	// The private key never appears in the output, and the key files are gone.
	if strings.Contains(out.String(), "PRIVATE KEY") {
		t.Errorf("output shows the private key:\n%s", out.String())
	}
	for _, c := range f.calls {
		if _, path, ok := strings.Cut(c, " -f "); ok && strings.HasPrefix(c, "exec ssh-keygen") {
			if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Errorf("key directory %s still exists (stat error: %v)", filepath.Dir(path), err)
			}
		}
	}
	if !strings.Contains(out.String(), "gorums-release cluster-key -prune") {
		t.Errorf("output = %q, want the next step to name cluster-key -prune", out.String())
	}
}

func TestClusterKeyAddKeepsBranchPolicy(t *testing.T) {
	f := keygenRunner(t)
	f.answers["deployment-branch-policies --jq"] = "7 release/v*\n"
	tl, _ := newTestTool(t, f)
	if err := tl.clusterKey(defaultClusterKeyOptions()); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.execs() {
		if strings.Contains(c, "--method POST") || strings.Contains(c, "--method DELETE") {
			t.Errorf("changed a branch policy that is already right: %s", c)
		}
	}
}

func TestClusterKeyAddRejectsBadPolicyID(t *testing.T) {
	f := keygenRunner(t)
	f.answers["deployment-branch-policies --jq"] = "x master\n"
	tl, _ := newTestTool(t, f)
	if err := tl.clusterKey(defaultClusterKeyOptions()); err == nil {
		t.Fatal("clusterKey() = nil, want an error for a policy id that is not a number")
	}
	for _, c := range f.calls {
		if strings.Contains(c, "DELETE") || strings.Contains(c, "gh secret") {
			t.Errorf("went on after a bad policy id: %s", c)
		}
	}
}

func TestClusterKeyAddRemovesOtherBranchPolicies(t *testing.T) {
	f := keygenRunner(t)
	f.answers["deployment-branch-policies --jq"] = "3 master\n7 release/v*\n9 v*\n"
	tl, _ := newTestTool(t, f)
	if err := tl.clusterKey(defaultClusterKeyOptions()); err != nil {
		t.Fatal(err)
	}
	var deleted []string
	for _, c := range f.execs() {
		if strings.Contains(c, "--method POST") {
			t.Errorf("added a branch policy that exists: %s", c)
		}
		if rest, ok := strings.CutPrefix(c, "gh api --method DELETE repos/{owner}/{repo}/environments/cluster/deployment-branch-policies/"); ok {
			deleted = append(deleted, rest)
		}
	}
	if strings.Join(deleted, ",") != "3,9" {
		t.Errorf("deleted policies %v, want [3 9]", deleted)
	}
	// The secret is set only after the environment admits release branches alone.
	assertOrder(t, f.calls,
		"exec gh api --method DELETE repos/{owner}/{repo}/environments/cluster/deployment-branch-policies/9",
		"exec gh secret set CLUSTER_SSH_KEY --env cluster",
	)
}

func TestClusterKeyAddStopsBeforeGitHub(t *testing.T) {
	f := keygenRunner(t)
	f.fail["ssh gorums@bb1 "] = errBoom
	tl, _ := newTestTool(t, f)
	if err := tl.clusterKey(defaultClusterKeyOptions()); err == nil {
		t.Fatal("clusterKey() = nil, want the ssh error")
	}
	for _, c := range f.calls {
		if strings.Contains(c, "gh secret") || strings.Contains(c, "gh api --method") {
			t.Errorf("changed GitHub after a failed key install: %s", c)
		}
	}
}

func TestClusterKeyAddDryRun(t *testing.T) {
	f := keygenRunner(t)
	f.fail["deployment-branch-policies --jq"] = errBoom // the environment does not exist yet
	f.answers["deployment-branch-policies --jq"] = `{"message":"Not Found","status":"404"}`
	f.onExec = nil                                       // a dry run must not need the key files
	tl, out := newTestTool(t, f)
	tl.dryRun = true
	if err := tl.clusterKey(defaultClusterKeyOptions()); err != nil {
		t.Fatal(err)
	}
	if len(f.execs()) != 0 {
		t.Errorf("dry run ran %v", f.execs())
	}
	if !strings.Contains(out.String(), "+ gh secret set CLUSTER_SSH_KEY --env cluster") {
		t.Errorf("dry run output = %q, want the skipped secret command", out.String())
	}
	if strings.Contains(out.String(), "DELETE") {
		t.Errorf("dry run plans to delete a policy that gh did not return:\n%s", out.String())
	}
}

func TestClusterKeyPrune(t *testing.T) {
	const before = "ssh-ed25519 AAAAown meling@laptop\n" +
		`from="x" ssh-ed25519 AAAAold gorums-release-smoke-2025-11-01` + "\n" +
		`from="x" ssh-ed25519 AAAAnew gorums-release-smoke-2026-11-11` + "\n"
	f := newFakeRunner()
	f.answers["cat .ssh/authorized_keys"] = before
	tl, out := newTestTool(t, f)
	o := defaultClusterKeyOptions()
	o.prune = true
	if err := tl.clusterKey(o); err != nil {
		t.Fatal(err)
	}
	assertOrder(t, f.calls,
		"query ssh gorums@ssh4.ux.uis.no cat .ssh/authorized_keys",
		"exec ssh gorums@ssh4.ux.uis.no ",
		"query ssh gorums@bb1 cat .ssh/authorized_keys",
		"exec ssh gorums@bb1 ",
	)
	want := "ssh-ed25519 AAAAown meling@laptop\n" +
		`from="x" ssh-ed25519 AAAAnew gorums-release-smoke-2026-11-11` + "\n"
	if got := stdinOf(t, f, "ssh gorums@bb1 "); got != want {
		t.Errorf("new authorized_keys = %q, want %q", got, want)
	}
	if !strings.Contains(out.String(), "gorums-release-smoke-2025-11-01") {
		t.Errorf("output = %q, want it to name the removed key", out.String())
	}
}

func TestClusterKeyPruneNothing(t *testing.T) {
	f := newFakeRunner()
	f.answers["cat .ssh/authorized_keys"] = `from="x" ssh-ed25519 AAAAnew gorums-release-smoke-2026-11-11` + "\n"
	tl, _ := newTestTool(t, f)
	o := defaultClusterKeyOptions()
	o.prune = true
	if err := tl.clusterKey(o); err != nil {
		t.Fatal(err)
	}
	if len(f.execs()) != 0 {
		t.Errorf("pruned with only one key: %v", f.execs())
	}
}

func TestPruneAuthorizedKeys(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		want        string
		wantRemoved []string
	}{
		{
			name: "no smoke keys",
			in:   "ssh-ed25519 AAAA a@b\n",
			want: "ssh-ed25519 AAAA a@b\n",
		},
		{
			name:        "keeps the newest and other keys",
			in:          "k1 gorums-release-smoke-2024-01-01\nown a@b\nk2 gorums-release-smoke-2026-11-11\nk3 gorums-release-smoke-2025-06-30\n",
			want:        "own a@b\nk2 gorums-release-smoke-2026-11-11\n",
			wantRemoved: []string{"gorums-release-smoke-2024-01-01", "gorums-release-smoke-2025-06-30"},
		},
		{
			name:        "same date keeps the last added",
			in:          "k1 gorums-release-smoke-2026-11-11\nown a@b\nk2 gorums-release-smoke-2026-11-11\n",
			want:        "own a@b\nk2 gorums-release-smoke-2026-11-11\n",
			wantRemoved: []string{"gorums-release-smoke-2026-11-11"},
		},
		{
			name: "keeps blank lines and comments",
			in:   "# keys\n\nk1 gorums-release-smoke-2026-11-11\n",
			want: "# keys\n\nk1 gorums-release-smoke-2026-11-11\n",
		},
		{
			name: "adds a final newline",
			in:   "own a@b",
			want: "own a@b\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, removed := pruneAuthorizedKeys(tt.in)
			if got != tt.want {
				t.Errorf("pruneAuthorizedKeys() = %q, want %q", got, tt.want)
			}
			if strings.Join(removed, ",") != strings.Join(tt.wantRemoved, ",") {
				t.Errorf("removed = %v, want %v", removed, tt.wantRemoved)
			}
		})
	}
}
