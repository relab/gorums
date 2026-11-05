#!/bin/bash
# Registers each line of MASK as a GitHub Actions log mask, so the job log hides
# it everywhere, also inside longer strings. Carriage returns and surrounding
# blanks are removed from each line, since a mask must match exactly. It fails
# when MASK holds no mask, so a missing secret stops the job before any step
# can print a cluster address.
set -euo pipefail

count=0
while IFS= read -r line; do
    line=${line//$'\r'/}
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    if [ -n "$line" ]; then
        echo "::add-mask::$line"
        count=$((count + 1))
    fi
done <<< "${MASK:-}"

if [ "$count" -eq 0 ]; then
    echo "::error::CLUSTER_MASK holds no mask; refusing to continue without hiding cluster details" >&2
    exit 1
fi
