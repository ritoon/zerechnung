#!/usr/bin/env bash
# Cross-language campaign: this repository's Go package against the
# TypeScript, Python, Rust, Java, C#, C, C++ and PHP libraries. See PROTOCOL.md.
#
#   ./run.sh setup              corpus (via bench/ecosystem), then each language's dependencies
#   ./run.sh survey    [lang…]  correctness surveys
#   ./run.sh bench     [lang…]  benchmarks, one language after the other
#   ./run.sh alloc     [lang…]  allocations of one call of each operation (go, typescript, python, rust)
#   ./run.sh resources [lang…]  CPU time and peak memory, one process per benchmark
#   ./run.sh report             tables in results/<date>/tables.md
#   ./run.sh all       [lang…]  all of the above in a row
#
# Languages: go typescript python rust java csharp c cpp php (all by default).
# Results go to results/<date>/ (OUT can be overridden); COUNT and BENCHTIME as
# in PROTOCOL.md (resources: RES_COUNT, 1 by default, and RES_BENCHTIME, 1).
set -euo pipefail
cd "$(dirname "$0")"
here=$(pwd)

LANGS=(go typescript python rust java csharp c cpp php)
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
		elif [ "$cmd" = alloc ] && [[ " typescript python rust " != *" $l "* ]]; then
			echo "   no allocation counter in the $l harness"
		elif [ "$l" = go ]; then
			# The Go harness takes BENCHTIME as a duration.
			(cd ../ecosystem && BENCHTIME="${BENCHTIME}s" ${awake[@]+"${awake[@]}"} ./run.sh "$cmd" facturx)
		else
			(cd "$l" && ${awake[@]+"${awake[@]}"} ./run.sh "$cmd")
		fi
	done
}

# resources measures, for each library and each benchmark, a process that runs
# that benchmark only: CPU time per operation over the timed series and the
# peak resident memory of the process (PROTOCOL.md). Records go to
# resources_<lang>.jsonl.
resources() {
	local l
	for l in $(langs "$@"); do
		echo "== resources $l"
		if [ "$l" = go ]; then
			# caffeinate cannot run a shell function: let it watch this shell.
			if [ ${#awake[@]} -gt 0 ]; then caffeinate -i -w $$ & fi
			COUNT=${RES_COUNT:-1} BENCHTIME=${RES_BENCHTIME:-1} go_resources
		else
			(cd "$l" && COUNT=${RES_COUNT:-1} BENCHTIME=${RES_BENCHTIME:-1} ${awake[@]+"${awake[@]}"} ./run.sh resources)
		fi
	done
}

# go_resources runs each benchmark of bench/ecosystem/facturx in its own test
# binary process, under /usr/bin/time -l for the peak RSS; FX_CPU makes the
# shared helpers report the CPU time per operation ("cpu-ns/op"). A second
# round with GOMAXPROCS=1 shows the package confined to one core (the GC then
# shares the measured thread's core).
go_resources() {
	local dir=../ecosystem/facturx tmp out="$OUT/resources_go.jsonl" ver b rss procs lib
	tmp=$(mktemp -d)
	(cd "$dir" && go test -c -o "$tmp/facturx.test" .)
	ver=$(go version | awk '{print $3}')
	: >"$out"
	for procs in "" 1; do
		lib="ZeRechnung (ce dépôt)"
		[ -n "$procs" ] && lib="ZeRechnung, GOMAXPROCS=1"
		for b in $(cd "$dir" && "$tmp/facturx.test" -test.list '^Benchmark' | sed 's/^Benchmark//'); do
			(cd "$dir" && env FX_CPU=1 ${procs:+GOMAXPROCS=$procs} /usr/bin/time -l -o "$tmp/time" "$tmp/facturx.test" -test.run '^$' \
				-test.bench "^Benchmark$b\$" -test.benchmem -test.count "$COUNT" -test.benchtime "${BENCHTIME}s") >"$tmp/out" 2>&1 || true
			rss=$(awk '/maximum resident set size/ {print $1}' "$tmp/time")
			awk -v b="$b" -v ver="$ver" -v rss="${rss:-0}" -v lib="$lib" '
				index($1, "Benchmark" b) == 1 && ($1 == "Benchmark" b || index($1, "Benchmark" b "-") == 1) {
					iters = $2
					for (i = 3; i < NF; i++) {
						if ($(i + 1) == "ns/op") ns = ns (ns == "" ? "" : ",") $i
						if ($(i + 1) == "cpu-ns/op") cpu = cpu (cpu == "" ? "" : ",") $i
						if ($(i + 1) == "B/op") bop = $i
						if ($(i + 1) == "allocs/op") aop = $i
					}
				}
				END {
					status = ns == "" ? "fail" : "ok"
					printf "{\"lang\":\"go\",\"lib\":\"%s\",\"version\":\"%s\",\"bench\":\"%s\",\"status\":\"%s\"", lib, ver, b, status
					if (ns != "") printf ",\"ns_per_op\":[%s],\"cpu_ns_per_op\":[%s],\"iters\":%s,\"bytes_per_op\":%s,\"allocs_per_op\":%s,\"max_rss_bytes\":%s", ns, cpu, iters, bop, aop, rss
					print "}"
				}' "$tmp/out" | tee -a "$out"
		done
	done
	rm -rf "$tmp"
	echo "== $out"
}

setup() {
	(cd ../ecosystem && ./run.sh setup)
	for l in "${LANGS[@]:1}"; do
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
		echo "java: $("${JAVA_HOME:-/opt/homebrew/opt/openjdk}/bin/java" -version 2>&1 | sed -n 1p)"
		echo "maven: $(mvn -v 2>/dev/null | head -1)"
		echo "dotnet: $(dotnet --version 2>/dev/null), $(dotnet --list-runtimes 2>/dev/null | grep Microsoft.NETCore.App | cut -d' ' -f1-2)"
		echo "cc: $(cc --version | head -1)"
		echo "cmake: $(cmake --version | head -1), ninja $(ninja --version 2>/dev/null)"
		echo "brew: $(brew list --versions mupdf qpdf libxml2 libxslt podofo poppler xerces-c pugixml 2>/dev/null | tr '\n' ',' | sed 's/,$//; s/,/, /g')"
		echo "php: $(php -r 'echo PHP_VERSION, PHP_ZTS ? " ZTS" : " NTS", ", libxml ", LIBXML_DOTTED_VERSION, ", opcache.enable_cli=", (int) ini_get("opcache.enable_cli");'), $(composer --version 2>/dev/null | head -1)"
		echo "pdfdetach: $(pdfdetach -v 2>&1 | head -1)"
		echo "count: $COUNT, benchtime: ${BENCHTIME}s"
	} >"$OUT/env.txt"
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup ;;
survey | bench) each "$cmd" "$@" ;;
alloc) each alloc "$@" ;;
resources) resources "$@" ;;
report) report ;;
all) setup && each survey "$@" && each bench "$@" && each alloc "$@" && resources "$@" && report ;;
*)
	sed -n '2,15p' "$0"
	exit 2
	;;
esac
cd "$here"
