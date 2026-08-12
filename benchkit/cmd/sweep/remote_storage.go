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

func ensureRemoteNamespace(ctx context.Context, host iago.Host, root string) (string, error) {
	namespace, err := remoteNamespace(ctx, host, root)
	if err != nil {
		return "", err
	}
	cmd := "test -d " + iago.Quote(root) + " && test -w " + iago.Quote(root) +
		" && mkdir -p " + iago.Quote(namespace) + " && test -w " + iago.Quote(namespace)
	if err := driverExec(ctx, host, cmd); err != nil {
		return "", fmt.Errorf("remote storage root %s on %s must exist and be writable: %w", root, host.Name(), err)
	}
	return namespace, nil
}
