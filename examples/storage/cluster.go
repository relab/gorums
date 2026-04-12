package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// runCluster spawns one server subprocess per address in the comma-separated
// addrs string, prints connection instructions, and waits for a signal before
// stopping all subprocesses. The ic string is the raw -interceptors flag value,
// forwarded to each subprocess unchanged.
func runCluster(addrs string, ic string) error {
	all := splitAddrs(addrs)
	if len(all) == 0 {
		return fmt.Errorf("no addresses provided")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	cmds := make([]*exec.Cmd, len(all))
	for i, addr := range all {
		// Put this server's address first, then the remaining peers.
		nodeArgs := addr + "," + strings.Join(append(all[:i:i], all[i+1:]...), ",")
		args := []string{"-serve", "-addrs", nodeArgs}
		if ic != "" {
			args = append(args, "-interceptors", ic)
		}
		cmd := exec.Command(exe, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			stopAll(cmds[:i])
			return fmt.Errorf("failed to start server %q: %w", addr, err)
		}
		cmds[i] = cmd
	}

	fmt.Printf("\nCluster running on %s.\n", strings.Join(all, ", "))
	fmt.Printf("Connect a client with:\n  %s -addrs %s\n\n", exe, addrs)
	fmt.Println("Press Ctrl-C to stop all servers.")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals

	fmt.Fprintln(os.Stderr, "\nStopping servers...")
	stopAll(cmds)
	return nil
}

// stopAll sends SIGTERM to all running processes and waits for them to exit.
func stopAll(cmds []*exec.Cmd) {
	for _, cmd := range cmds {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	for _, cmd := range cmds {
		if cmd != nil {
			_ = cmd.Wait()
		}
	}
}
