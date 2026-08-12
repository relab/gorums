package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/relab/iago"
)

func runDriverList(cfg *config) error {
	if cfg.driver == "" {
		return errors.New("-list requires -driver or a saved driver in .sweep-last.json")
	}
	group, err := dialDriverGroup(cfg.driver, cfg.sshConfig)
	if err != nil {
		return fmt.Errorf("connect to driver: %w", err)
	}
	defer group.Close()
	host := group.Hosts[0]
	latest := ""
	namespace := ""
	if state, err := readLastRunState(cfg.rootDir); err == nil && state.Driver == cfg.driver {
		latest = state.RemoteWorkDir
		namespace = state.RemoteNamespace
	}
	if namespace == "" {
		namespace, err = remoteNamespace(context.Background(), host, cfg.remoteDir)
		if err != nil {
			return err
		}
	}
	script := `set -eu
ns=$1
[ -d "$ns" ] || exit 0
find "$ns" -mindepth 1 -maxdepth 1 -type d -name 'sweep-driver-*' -print0 |
  xargs -0 -r ls -1dt |
  while IFS= read -r wd; do
    status=recoverable
    exit_code=-
    [ -f "$wd/exit.code" ] && exit_code=$(cat "$wd/exit.code")
    if [ ! -f "$wd/exit.code" ]; then
      status=active
    elif [ -f "$wd/compact.collected" ]; then
      status=raw-pending
    elif find "$wd/out" -type d -name '` + compactTransferDir + `' -print -quit 2>/dev/null | grep -q .; then
      status=completed
    fi
    label=$(sed -n 's/.*"label":"\([^"]*\)".*/\1/p' "$wd/run.meta.json" 2>/dev/null)
    [ -n "$label" ] || label=$(basename "$wd")
    started=$(sed -n 's/.*"launched_at":"\([^"]*\)".*/\1/p' "$wd/run.meta.json" 2>/dev/null)
    [ -n "$started" ] || started=$(stat -c %y "$wd" 2>/dev/null | cut -d. -f1 || stat -f '%Sm' -t '%Y-%m-%d %H:%M:%S' "$wd")
    size=$(du -sh "$wd" 2>/dev/null | awk '{print $1}')
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$started" "$label" "$status" "$exit_code" "$size" "$wd"
  done
`
	out, err := iago.Output(context.Background(), host, "sh -c "+iago.Quote(script)+" sh "+iago.Quote(namespace))
	if err != nil {
		return err
	}
	return writeDriverList(os.Stdout, cfg.driver, namespace, latest, out)
}

// writeDriverList renders tab-delimited driver run data as aligned columns.
func writeDriverList(w io.Writer, driver, namespace, latest, rows string) error {
	if _, err := fmt.Fprintf(w, "DRIVER %s  NAMESPACE %s\n", driver, namespace); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "STARTED\tLABEL\tSTATUS\tEXIT\tSIZE\tPATH"); err != nil {
		return err
	}
	for line := range strings.SplitSeq(strings.TrimSpace(rows), "\n") {
		if line == "" {
			continue
		}
		if started, rest, ok := strings.Cut(line, "\t"); ok {
			line = formatRunListTimestamp(started) + "\t" + rest
		}
		if latest != "" && strings.HasSuffix(line, "\t"+latest) {
			line += "  (latest)"
		}
		if _, err := fmt.Fprintln(tw, line); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatRunListTimestamp(timestamp string) string {
	started, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return timestamp
	}
	return started.Format("2006-01-02 15:04:05")
}
