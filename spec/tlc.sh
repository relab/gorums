#!/usr/bin/env bash
# Runs TLC on the model configurations in spec/.
#
# Usage:
#   spec/tlc.sh list
#   spec/tlc.sh run CONFIG [TLC options]
#   spec/tlc.sh check [-all] [CONFIG ...]
#   spec/tlc.sh summary CONFIG
#
# CONFIG is a configuration name, such as Current; a model and a name, such as
# stream/Current; or a path to a .cfg file.
#
# list prints each model's configurations, with their run time, expected
# result, and description.
# run checks one configuration. It shows TLC's progress, then the summary.
# check runs configurations and compares each result with the configuration's
# "\* Expect:" line: "pass", "deadlock", or "violates P". Without
# configurations, it runs each one whose "\* Run:" line is "short"; -all adds
# the long ones. Its exit status is 1 if any result differs.
# summary prints the summary of a configuration's saved TLC output: the result
# and, for an error, the trace one line per step, with only the variables that
# changed. CONFIG may also be the path to a saved output file.
#
# Each model is a folder in spec/. Each configuration starts with
# "\* Module: M", which names the module that TLC checks. TLC output goes to
# out/Name.out next to the configuration, and TLC keeps its state files in
# out/states/Name.
# The script takes tla2tools.jar from $TLA2TOOLS, or else from the TLA+ VS Code
# extension.
set -euo pipefail

SPEC=$(cd "$(dirname "$0")" && pwd)

usage() {
    sed -n 's/^#   //p' "$0" | sed 's/^/usage: /' >&2
    exit 2
}

# field prints the value of the "\* Name:" header line in a configuration.
field() {
    sed -n "s/^\\\\\\* $1: *//p" "$2" | head -1
}

jar() {
    local j found=${TLA2TOOLS:-}
    if [ -z "$found" ]; then
        for j in "$HOME"/.vscode/extensions/tlaplus.vscode-ide-*/tools/tla2tools.jar; do
            if [ -f "$j" ]; then found=$j; fi
        done
    fi
    if [ ! -f "$found" ]; then
        echo "tla2tools.jar not found: set TLA2TOOLS to its path, or install the TLA+ VS Code extension." >&2
        echo "Releases: https://github.com/tlaplus/tlaplus/releases" >&2
        exit 2
    fi
    echo "$found"
}

# resolve prints the path of the configuration that CONFIG names.
resolve() {
    local arg=$1 name=${1%.cfg} c
    for c in "$arg" "$SPEC/$name.cfg"; do
        if [ -f "$c" ] && [ "${c%.cfg}" != "$c" ]; then
            echo "$c"
            return
        fi
    done
    local found=()
    for c in "$SPEC"/*/"$name".cfg; do
        if [ -f "$c" ]; then found+=("$c"); fi
    done
    case ${#found[@]} in
    1) echo "${found[0]}" ;;
    0)
        echo "no configuration named $arg; spec/tlc.sh list shows them" >&2
        return 2
        ;;
    *)
        echo "$arg is ambiguous; name the model, as in:" >&2
        for c in "${found[@]}"; do echo "  $(basename "$(dirname "$c")")/$name" >&2; done
        return 2
        ;;
    esac
}

# outfile prints the path of the TLC output file for a configuration.
outfile() {
    echo "$(cd "$(dirname "$1")" && pwd)/out/$(basename "$1" .cfg).out"
}

# tlc runs TLC on one configuration and saves its output. With show=true, it
# also prints TLC's progress lines on stderr. It returns TLC's exit status.
tlc() {
    local show=$1 cfg=$2
    shift 2
    local dir name module out status
    dir=$(cd "$(dirname "$cfg")" && pwd)
    name=$(basename "$cfg" .cfg)
    module=$(field Module "$dir/$name.cfg")
    if [ -z "$module" ]; then
        echo "$cfg: no '\\* Module:' line" >&2
        return 2
    fi
    out="$dir/out/$name.out"
    mkdir -p "$dir/out"
    set +e
    (cd "$dir" && exec java -XX:+UseParallelGC -cp "$JAR" tlc2.TLC -workers auto \
        -noGenerateSpecTE -config "$name.cfg" -metadir "out/states/$name" "$@" "$module.tla") 2>&1 |
        if $show; then
            tee "$out" | awk '/^Progress\(/ { print; fflush() }' >&2
        else
            cat > "$out"
        fi
    status=${PIPESTATUS[0]}
    set -e
    return "$status"
}

# summary prints the TLC result and, for an error, the trace one line per step
# with the variables that changed, sorted by name.
summary() {
    awk '
    function flush(   n, i, j, k, t, s, keys) {
        if (!step) return
        if (!started) {
            printf "%3s %s\n", num, act
        } else {
            n = 0
            for (k in cur) if (!(k in prev) || prev[k] != cur[k]) keys[++n] = k
            for (i = 2; i <= n; i++) {
                t = keys[i]
                for (j = i - 1; j >= 1 && keys[j] > t; j--) keys[j + 1] = keys[j]
                keys[j + 1] = t
            }
            s = ""
            for (i = 1; i <= n; i++) s = s (i > 1 ? "  " : "") keys[i] "=" cur[keys[i]]
            printf "%3s %-16s %s\n", num, act, s
        }
        delete prev
        for (k in cur) prev[k] = cur[k]
        delete cur
        ncur = 0
        started = 1
    }
    /^Error:/ { print; trace = 1; next }
    !trace {
        if (/^Model checking completed/ || /^Finished in/ ||
            (/distinct states found/ && /left on queue/ && !/^Progress/)) print
        next
    }
    /^State [0-9]+: </ || /Stuttering/ || /^Back to state/ || /^Finished/ {
        isstate = /^State [0-9]+: </
        flush()
        if (isstate) {
            num = $2
            sub(/:$/, "", num)
            act = $0
            sub(/^State [0-9]+: </, "", act)
            sub(/[ >].*/, "", act)
            step = 1
        } else {
            step = 0
            print
        }
        next
    }
    step && /^\/\\ [A-Za-z0-9_]+ = / {
        last = $2
        v = $0
        sub(/^\/\\ [A-Za-z0-9_]+ = /, "", v)
        cur[last] = v
        ncur++
        next
    }
    step && ncur && /^ / {
        v = $0
        sub(/^ +/, "", v)
        cur[last] = cur[last] " " v
    }
    END { flush() }
    ' "$@"
}

cmd_list() {
    local m c
    for m in "$SPEC"/*/; do
        m=${m%/}
        printf 'spec/%s\n' "$(basename "$m")"
        printf '  %-24s %-6s %-30s %s\n' NAME RUN EXPECT DESCRIPTION
        for c in "$m"/*.cfg; do
            if [ ! -f "$c" ]; then continue; fi
            printf '  %-24s %-6s %-30s %s\n' "$(basename "$c" .cfg)" "$(field Run "$c")" \
                "$(field Expect "$c")" \
                "$(awk '/^\\\* / && !/^\\\* (Module|Expect|Run):/ { sub(/^\\\* /, ""); print; exit }' "$c")"
        done
    done
}

cmd_run() {
    [ $# -ge 1 ] || usage
    local cfg status=0
    cfg=$(resolve "$1")
    shift
    tlc true "$cfg" "$@" || status=$?
    summary "$(outfile "$cfg")"
    echo "Full output: $(outfile "$cfg")"
    return "$status"
}

cmd_check() {
    local all=false c name expect pat out start took got failed=0
    if [ "${1:-}" = "-all" ]; then
        all=true
        shift
    fi
    local cfgs=()
    if [ $# -gt 0 ]; then
        for c in "$@"; do
            c=$(resolve "$c")
            cfgs+=("$c")
        done
    else
        for c in "$SPEC"/*/*.cfg; do
            if $all || [ "$(field Run "$c")" = short ]; then
                cfgs+=("$c")
            fi
        done
    fi
    if [ ${#cfgs[@]} -eq 0 ]; then
        echo "no configurations to check" >&2
        return 2
    fi
    for c in "${cfgs[@]}"; do
        name=$(basename "$c" .cfg)
        expect=$(field Expect "$c")
        case $expect in
        pass) pat='^Model checking completed\. No error has been found' ;;
        deadlock) pat='^Error: Deadlock reached' ;;
        "violates "*) pat="^Error: (Invariant|Temporal property) ${expect#violates } (is|was) violated" ;;
        *)
            printf 'FAIL  %-28s bad Expect line: "%s"\n' "$name" "$expect"
            failed=1
            continue
            ;;
        esac
        start=$SECONDS
        tlc false "$c" || true
        took=$((SECONDS - start))
        out=$(outfile "$c")
        if grep -Eq "$pat" "$out"; then
            printf 'ok    %-28s %-32s %4ds\n' "$name" "$expect" "$took"
        else
            got=$(grep -E -m1 '^Error:|^Model checking completed' "$out" || echo "no result; see $out")
            printf 'FAIL  %-28s want %s; got: %s\n' "$name" "$expect" "$got"
            failed=1
        fi
    done
    return $failed
}

[ $# -ge 1 ] || usage
cmd=$1
shift
case $cmd in
list) cmd_list ;;
run)
    JAR=$(jar)
    cmd_run "$@"
    ;;
check)
    JAR=$(jar)
    cmd_check "$@"
    ;;
summary)
    [ $# -eq 1 ] || usage
    if [ -f "$1" ] && [ "${1%.cfg}" = "$1" ]; then
        summary "$1"
    else
        cfg=$(resolve "$1")
        summary "$(outfile "$cfg")"
    fi
    ;;
*) usage ;;
esac
