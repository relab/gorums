#!/usr/bin/env bash
# fix-host-keys.sh — Re-learn SSH host keys for re-imaged cluster nodes.
#
# When a node is re-imaged its host key changes, and the gorums sweep tool then
# fails with "knownhosts: key mismatch": iago verifies host keys against
# known_hosts but cannot rewrite that file itself. This script repairs the named
# hosts from the outside using the system ssh and ssh-keygen. For each host it
# removes the stale known_hosts entry (ssh-keygen -R keeps a .old backup) and
# reconnects with StrictHostKeyChecking=accept-new to learn the current key and
# confirm the host is reachable.
#
# It only ever touches the hosts you name on the command line, and it never
# re-learns the jump host: that gateway is a stable trust anchor that is not
# re-imaged. accept-new adds unknown keys but refuses on mismatch, so a changed
# gateway key fails loudly rather than being replaced silently; repair it once
# by hand if that ever happens.
#
# Usage:
#   ./scripts/fix-host-keys.sh [-F ssh_config] host [host ...]
#   ./scripts/fix-host-keys.sh bb{1..30}     # the shell expands the range
#
# Pass the same -F ssh_config you pass to the sweep tool, so the hostnames
# resolve identically.

set -uo pipefail

ssh_config=""
if [[ "${1:-}" == "-F" ]]; then
    ssh_config="${2:-}"
    [[ -n "$ssh_config" ]] || { echo "error: -F requires a config file" >&2; exit 2; }
    shift 2
fi

if [[ $# -eq 0 ]]; then
    echo "Usage: $0 [-F ssh_config] host [host ...]" >&2
    echo "  e.g. $0 bb{1..30}" >&2
    exit 2
fi

ssh_opt=()
[[ -n "$ssh_config" ]] && ssh_opt=(-F "$ssh_config")

# resolved_hostname prints the HostName ssh uses for an alias; known_hosts is
# keyed by this (e.g. bb1.ux.uis.no, not bb1). Falls back to the alias.
resolved_hostname() {
    local h
    h=$(ssh "${ssh_opt[@]}" -G "$1" 2>/dev/null | awk 'tolower($1)=="hostname"{print $2; exit}')
    echo "${h:-$1}"
}

echo "re-learning host keys for $# host(s)..."
failed=()
for alias in "$@"; do
    hostname=$(resolved_hostname "$alias")

    # Drop stale entries (a no-op if absent; ssh-keygen keeps a .old backup).
    ssh-keygen -R "$hostname" >/dev/null 2>&1
    [[ "$alias" != "$hostname" ]] && ssh-keygen -R "$alias" >/dev/null 2>&1

    # Reconnect on a fresh, unmultiplexed connection to learn the current key.
    if out=$(ssh "${ssh_opt[@]}" \
        -o BatchMode=yes \
        -o StrictHostKeyChecking=accept-new \
        -o ControlMaster=no -o ControlPath=none \
        -o ConnectTimeout=15 \
        "$alias" true 2>&1); then
        echo "  ok   $alias ($hostname)"
    else
        echo "  FAIL $alias ($hostname): $(echo "$out" | tr '\n' ' ')"
        failed+=("$alias")
    fi
done

if (( ${#failed[@]} > 0 )); then
    echo "${#failed[@]} host(s) failed: ${failed[*]}" >&2
    exit 1
fi
echo "all $# host key(s) re-learned"
