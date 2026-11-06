package main

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/relab/iago"
)

var remoteUserPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func remoteNamespace(ctx context.Context, host iago.Host, root string) (string, error) {
	user := host.GetEnv("USER")
	var err error
	if strings.TrimSpace(user) == "" {
		user, err = iago.Output(ctx, host, "id -un")
	}
	user = strings.TrimSpace(user)
	if err != nil {
		return "", fmt.Errorf("determine remote user on %s: %w", host.Name(), err)
	}
	if !remoteUserPattern.MatchString(user) {
		return "", fmt.Errorf("unsafe remote user %q on %s", user, host.Name())
	}
	return path.Join(root, "sweep-"+user), nil
}

// ensureRemoteNamespace creates the remote user's private namespace below root
// on host, or checks an existing one, and returns its path. See
// [namespaceCommand] for what makes a namespace safe to use.
func ensureRemoteNamespace(ctx context.Context, host iago.Host, root string) (string, error) {
	namespace, err := remoteNamespace(ctx, host, root)
	if err != nil {
		return "", err
	}
	if err := driverExec(ctx, host, namespaceCommand(root, namespace)); err != nil {
		return "", fmt.Errorf("remote storage root %s on %s must exist and be writable, and %s must be a directory owned by the remote user: %w",
			root, host.Name(), namespace, err)
	}
	return namespace, nil
}

// namespaceCommand returns the shell command that makes namespace, a directory
// in root, safe to use. Sweep runs cached executables from the namespace, and
// root is often a shared directory such as /tmp, so the command fails unless
// root is a writable directory and the namespace is a directory, not a
// symlink, that the remote user owns. It creates a missing namespace with mode
// 0700 and sets an existing one to 0700, so no other user can replace its
// files.
func namespaceCommand(root, namespace string) string {
	r, ns := iago.Quote(root), iago.Quote(namespace)
	return "test -d " + r + " && test -w " + r +
		" && { test -e " + ns + " || test -L " + ns + " || mkdir -m 700 " + ns + "; }" +
		" && test -d " + ns + " && ! test -L " + ns + " && test -O " + ns +
		" && chmod 700 " + ns
}
