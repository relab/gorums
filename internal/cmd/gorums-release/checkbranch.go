package main

import "fmt"

// checkBranchOptions holds the flags of the check-branch command.
type checkBranchOptions struct {
	branch string // the branch to check; empty means the checked-out branch
}

// checkBranch fails unless the branch is release/<version>, where <version> is
// the version in internal/version/version.go. The release smoke workflow runs
// it, so that its release-only jobs build only a release candidate commit.
func (t *tool) checkBranch(o checkBranchOptions) error {
	branch := o.branch
	if branch == "" {
		b, err := t.currentBranch()
		if err != nil {
			return err
		}
		branch = b
	}
	v, err := t.currentVersion()
	if err != nil {
		return err
	}
	if want := "release/" + v.String(); branch != want {
		return fmt.Errorf("%s is not the release branch for %s (want %s)", branch, v, want)
	}
	t.logf("%s matches %s", branch, v)
	return nil
}
