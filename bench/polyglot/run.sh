#!/usr/bin/env bash
# Cross-language campaign: this repository's Go package against the
# TypeScript, Python and Rust libraries. See PROTOCOL.md.
#
#   ./run.sh setup            corpus (via bench/ecosystem), then each language's dependencies
#   ./run.sh survey [lang…]   correctness surveys
#   ./run.sh bench  [lang…]   benchmarks, one language after the other
#   ./run.sh alloc  [lang…]   allocations of one call of each operation
#   ./run.sh report           tables in results/<date>/tables.md
#   ./run.sh all    [lang…]   the five in a row
#
# Languages: go typescript python rust (all by default). Results go to
# results/<date>/ (OUT can be overridden); COUNT and BENCHTIME as in PROTOCOL.md.
set -euo pipefail
cd "$(dirname "$0")"
here=$(pwd)

LANGS=(go typescript python rust)
OUT=${OUT:-results/$(date +%F)}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
export OUT COUNT=${COUNT:-3} BENCHTIME=${BENCHTIME:-1}

langs() { if [ $# -gt 0 ]; then echo "$@"; else echo "${LANGS[@]}"; fi; }

# macOS: keep the machine from idle-sleeping during a campaign (a sleep in
# the middle of a series ruins it). It does not prevent a lid-close sleep.
awake=()
command -v caffeinate >/dev/null && awake=(caffeinate -i)

each() {
	local cmd=$1
	shift
	for l in $(langs "$@"); do
		echo "== $cmd $l"
		if [ "$l" = go ] && [ "$cmd" = alloc ]; then
			echo "   go test -benchmem gives them with the benchmarks (bench)"
		elif [ "$l" = go ]; then
			# The Go harness takes BENCHTIME as a duration.
			(cd ../ecosystem && BENCHTIME="${BENCHTIME}s" ${awake[@]+"${awake[@]}"} ./run.sh "$cmd" facturx)
		else
			(cd "$l" && ${awake[@]+"${awake[@]}"} ./run.sh "$cmd")
		fi
	done
}

setup() {
	(cd ../ecosystem && ./run.sh setup)
	for l in typescript python rust; do
		echo "== setup $l"
		(cd "$l" && ./run.sh setup)
	done
}

report() {
	(cd report && go run . "$OUT") >"$OUT/tables.md" && echo "== $OUT/tables.md"
	{
		echo "date: $(date -u +%FT%TZ)"
		echo "cpu: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || uname -m)"
		echo "os: $(uname -sr)"
		echo "go: $(go version)"
		echo "node: $(node --version)"
		[ -x python/.venv/bin/python ] && echo "python: $(python/.venv/bin/python --version 2>&1)"
		echo "rust: $(rustc --version)"
		echo "count: $COUNT, benchtime: ${BENCHTIME}s"
	} >"$OUT/env.txt"
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup ;;
survey | bench) each "$cmd" "$@" ;;
alloc) each alloc "$@" ;;
report) report ;;
all) setup && each survey "$@" && each bench "$@" && each alloc "$@" && report ;;
*)
	sed -n '2,13p' "$0"
	exit 2
	;;
esac
cd "$here"
