package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The cluster access that the release smoke workflow uses. The jump host only
// forwards SSH to the driver, whose HostName is in
// .github/actions/cluster-ssh/ssh_config; the cluster accepts the key only from
// the jump host and the cluster network.
const (
	clusterEnv             = "cluster"
	clusterSecret          = "CLUSTER_SSH_KEY"
	clusterBranches        = "release/v*"
	clusterKeyPrefix       = "gorums-release-smoke-"
	jumpKeyRestrictions    = `command="/bin/false",restrict,port-forwarding,permitopen="bb1.ux.uis.no:22"`
	clusterKeyRestrictions = `from="152.94.1.60,152.94.162.0/26",restrict,port-forwarding,agent-forwarding,pty`
)

// Remote scripts, run by the login shell of the account. Each reads its input
// from stdin and runs under sh, whatever the login shell is.
const (
	// addKeyScript appends the line on stdin to authorized_keys, unless the
	// file already has it.
	addKeyScript = `sh -c 'umask 077 && mkdir -p .ssh && IFS= read -r line && touch .ssh/authorized_keys && { grep -qxF -- "$line" .ssh/authorized_keys || printf "%s\n" "$line" >> .ssh/authorized_keys; }'`
	// writeKeysScript replaces authorized_keys with stdin.
	writeKeysScript = `sh -c 'umask 077 && cat > .ssh/authorized_keys.new && mv .ssh/authorized_keys.new .ssh/authorized_keys'`
)

// clusterKeyOptions holds the flags of the cluster-key command.
type clusterKeyOptions struct {
	prune   bool   // remove the older keys instead of adding a new one
	jump    string // the ssh destination of the account on the jump host
	cluster string // the ssh destination of the account on the cluster
}

// clusterKey adds a new key for the release smoke workflow, or with prune,
// removes the older ones.
func (t *tool) clusterKey(o clusterKeyOptions) error {
	if o.prune {
		return t.pruneClusterKeys(o)
	}
	return t.addClusterKey(o)
}

// addClusterKey creates a key pair, installs the public key on the jump host
// and the cluster, sets up the cluster environment with the private key as its
// secret, and requires approval of workflows from outside collaborators. The
// older keys stay installed. The private key exists on disk only while
// ssh-keygen and gh need it.
func (t *tool) addClusterKey(o clusterKeyOptions) error {
	for _, name := range []string{"ssh-keygen", "ssh", "gh"} {
		if _, err := t.lookPath(name); err != nil {
			return fmt.Errorf("%s is required but is not on PATH", name)
		}
	}
	if _, err := t.query("gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not logged in: run gh auth login: %w", err)
	}

	comment := clusterKeyPrefix + t.now().Format("2006-01-02")
	pub, priv, err := t.newKeyPair(comment)
	if err != nil {
		return err
	}
	t.logf("Installing key %s for %s and %s.", comment, o.jump, o.cluster)
	if err := t.execCmd(cmd{name: "ssh", args: []string{o.jump, addKeyScript}, stdin: jumpKeyRestrictions + " " + pub + "\n"}); err != nil {
		return err
	}
	if err := t.execCmd(cmd{name: "ssh", args: []string{o.cluster, addKeyScript}, stdin: clusterKeyRestrictions + " " + pub + "\n"}); err != nil {
		return err
	}

	if err := t.setupClusterEnv(); err != nil {
		return err
	}
	if err := t.execCmd(cmd{name: "gh", args: []string{"secret", "set", clusterSecret, "--env", clusterEnv}, stdin: priv}); err != nil {
		return err
	}
	if err := t.exec("gh", "api", "--method", "PUT", "repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval",
		"-f", "approval_policy=all_external_contributors"); err != nil {
		return err
	}

	t.logf(`
Next, check the new key with the cluster-check workflow:

  git push origin HEAD:refs/heads/release/v0-probe

Approve the %s environment in the Actions tab and wait for the job. Then:

  git push origin --delete release/v0-probe
  %s cluster-key -prune`, clusterEnv, progName)
	return nil
}

// newKeyPair creates an ed25519 key pair without a passphrase and returns the
// public key line and the private key. A dry run returns placeholders.
func (t *tool) newKeyPair(comment string) (pub, priv string, err error) {
	dir, err := os.MkdirTemp("", "gorums-cluster-key-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "id_ed25519")
	if err := t.exec("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", path); err != nil {
		return "", "", err
	}
	if t.dryRun {
		return "ssh-ed25519 <new public key> " + comment, "<new private key>", nil
	}
	pubBytes, err := os.ReadFile(path + ".pub")
	if err != nil {
		return "", "", err
	}
	privBytes, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(pubBytes)), string(privBytes), nil
}

// setupClusterEnv creates or updates the cluster environment: the gh user is
// its required reviewer, and only release branches may deploy to it.
func (t *tool) setupClusterEnv() error {
	out, err := t.query("gh", "api", "user", "--jq", ".id")
	if err != nil {
		return err
	}
	var id int64
	if _, err := fmt.Sscan(out, &id); err != nil {
		return fmt.Errorf("cannot read the gh user id from %q: %w", out, err)
	}
	type reviewer struct {
		Type string `json:"type"`
		ID   int64  `json:"id"`
	}
	type branchPolicy struct {
		ProtectedBranches    bool `json:"protected_branches"`
		CustomBranchPolicies bool `json:"custom_branch_policies"`
	}
	body, err := json.Marshal(struct {
		Reviewers              []reviewer   `json:"reviewers"`
		DeploymentBranchPolicy branchPolicy `json:"deployment_branch_policy"`
	}{
		Reviewers:              []reviewer{{Type: "User", ID: id}},
		DeploymentBranchPolicy: branchPolicy{CustomBranchPolicies: true},
	})
	if err != nil {
		return err
	}
	envPath := "repos/{owner}/{repo}/environments/" + clusterEnv
	if err := t.execCmd(cmd{name: "gh", args: []string{"api", "--method", "PUT", envPath, "--input", "-"}, stdin: string(body)}); err != nil {
		return err
	}

	// Each line of the answer is "<policy id> <pattern>". Every policy with
	// another pattern goes, so that only release branches get the secret.
	policiesPath := envPath + "/deployment-branch-policies"
	// A dry run does not create the environment, so the query can fail; it then
	// plans as if the environment had no policies.
	out, err = t.query("gh", "api", policiesPath, "--jq", `.branch_policies[] | "\(.id) \(.name)"`)
	if err != nil {
		if !t.dryRun {
			return err
		}
		out = ""
	}
	var found bool
	for line := range strings.Lines(out) {
		id, name, _ := strings.Cut(strings.TrimSpace(line), " ")
		if _, err := strconv.ParseUint(id, 10, 64); err != nil {
			return fmt.Errorf("unexpected deployment branch policy %q from gh", strings.TrimSpace(line))
		}
		if name == clusterBranches {
			found = true
			continue
		}
		if err := t.exec("gh", "api", "--method", "DELETE", policiesPath+"/"+id); err != nil {
			return err
		}
	}
	if found {
		return nil
	}
	return t.exec("gh", "api", "--method", "POST", policiesPath,
		"-f", "name="+clusterBranches, "-f", "type=branch")
}

// pruneClusterKeys removes all but the newest release smoke key from
// authorized_keys on the jump host and the cluster.
func (t *tool) pruneClusterKeys(o clusterKeyOptions) error {
	if _, err := t.lookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is required but is not on PATH")
	}
	for _, dest := range []string{o.jump, o.cluster} {
		before, err := t.query("ssh", dest, "cat .ssh/authorized_keys")
		if err != nil {
			return err
		}
		after, removed := pruneAuthorizedKeys(before)
		if len(removed) == 0 {
			t.logf("%s: no older key to remove.", dest)
			continue
		}
		t.logf("%s: removing %s.", dest, strings.Join(removed, ", "))
		if err := t.execCmd(cmd{name: "ssh", args: []string{dest, writeKeysScript}, stdin: after}); err != nil {
			return err
		}
	}
	return nil
}

// pruneAuthorizedKeys returns the authorized_keys content with only the newest
// release smoke key, and the comments of the removed keys. The key comment
// holds the key's date, so the newest key has the greatest comment; of keys
// with the same date, the last line is the newest, because cluster-key appends.
// Other lines are kept as they are.
func pruneAuthorizedKeys(content string) (string, []string) {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	smokeComment := func(line string) string {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			return ""
		}
		if c := fields[len(fields)-1]; strings.HasPrefix(c, clusterKeyPrefix) {
			return c
		}
		return ""
	}
	newest := -1 // index of the newest key's line
	for i, line := range lines {
		if c := smokeComment(line); c != "" && (newest < 0 || c >= smokeComment(lines[newest])) {
			newest = i
		}
	}
	var kept, removed []string
	for i, line := range lines {
		if c := smokeComment(line); c != "" && i != newest {
			removed = append(removed, c)
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n") + "\n", removed
}
