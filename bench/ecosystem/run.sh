#!/usr/bin/env bash
# Benchmarks of the Go Factur-X ecosystem. See README.md.
#
#   ./run.sh setup              fetch the corpus and dependencies, build the host PDFs
#   ./run.sh survey [module…]   correctness surveys on the whole corpus
#   ./run.sh bench  [module…]   benchmarks (COUNT and BENCHTIME can be overridden)
#   ./run.sh report             tables in results/<date>/tables.md
#   ./run.sh all    [module…]   setup + survey + bench + report
#
# Results go to results/<date>/ (OUT can be overridden).
set -euo pipefail
cd "$(dirname "$0")"

CORPUS_URL=https://github.com/ZUGFeRD/corpus.git
CORPUS_COMMIT=84f8d8f68f6c3698aa3019d701a8b82db9ba0aac
FX_URL=https://github.com/solarpush/fX.git
FX_COMMIT=1314f3d94a8092ab94a2db8909f07f00e0eab875

MODULES=(floor facturx pdfcpu speedata andee dotwave goblde angelo papierleicht kanjo dcnws pdf0 litho taha fx gopdfattach)
OUT=${OUT:-results/$(date +%F)}
COUNT=${COUNT:-3}
BENCHTIME=${BENCHTIME:-1s}

# One module per library: -mod=mod lets go resolve them without a workspace,
# GOTOOLCHAIN=auto downloads Go 1.27.x for the modules that require it.
export GOFLAGS=-mod=mod GOTOOLCHAIN=auto

setup() {
	if [ ! -d corpus/.git ]; then
		echo "== corpus ZUGFeRD (sparse, ${CORPUS_COMMIT:0:7})"
		git clone --quiet --filter=blob:none --no-checkout "$CORPUS_URL" corpus
		git -C corpus sparse-checkout set XML-Rechnung ZUGFeRDv1/correct ZUGFeRDv2/correct
		git -C corpus checkout --quiet "$CORPUS_COMMIT"
	fi
	# The module zip of solarpush/fX is rejected by the go command (a file name
	# contains a carriage return), so the fx module builds from a local clone.
	if [ ! -d .deps/solarpush-fx/.git ]; then
		echo "== solarpush/fX (${FX_COMMIT:0:7})"
		git clone --quiet "$FX_URL" .deps/solarpush-fx
		git -C .deps/solarpush-fx checkout --quiet "$FX_COMMIT"
	fi
	echo "== PDF hôtes (host.pdf, host_classic.pdf)"
	(cd pdfcpu && go test -count 1 -run '^(TestMakeHost|TestMakeHostClassic|TestHostsStrict)$' . >/dev/null)
}

modules() { if [ $# -gt 0 ]; then echo "$@"; else echo "${MODULES[@]}"; fi; }

survey() {
	mkdir -p "$OUT"
	for m in $(modules "$@"); do
		[ "$m" = floor ] && continue
		echo "== survey $m"
		(cd "$m" && go test -count 1 -run '^TestSurvey$' -v . 2>&1 || true) |
			grep -E 'SURVEY|FLAG|MISS|PARSEFAIL' | sed -E 's/^ +[a-z_]+\.go:[0-9]+: //' >"$OUT/survey_$m.txt" || true
		grep SURVEY "$OUT/survey_$m.txt" || echo "   (aucun résultat)"
	done
}

bench() {
	mkdir -p "$OUT"
	for m in $(modules "$@"); do
		echo "== bench $m"
		# Some benchmarks fail on purpose (a library returns a PDF that does not
		# read back): keep going and record what was measured.
		{ (cd "$m" && go version && go test -run '^$' -bench . -benchmem -count "$COUNT" -benchtime "$BENCHTIME" . 2>&1) || true; } >"$OUT/bench_$m.txt"
		grep -E -- '--- FAIL|^FAIL' "$OUT/bench_$m.txt" || true
	done
}

report() {
	local abs
	abs=$(cd "$OUT" && pwd)
	(cd report && go run . "$abs") >"$abs/tables.md" && echo "== $OUT/tables.md"
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup ;;
survey) survey "$@" ;;
bench) bench "$@" ;;
report) report ;;
all) setup && survey "$@" && bench "$@" && report ;;
*)
	sed -n '2,10p' "$0"
	exit 2
	;;
esac
