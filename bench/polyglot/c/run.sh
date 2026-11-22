#!/usr/bin/env bash
# C part of the cross-language benchmark. See ../PROTOCOL.md and COVERAGE.md.
#
#   ./run.sh setup            official artefacts (SHA-256 checked), CMake Release build
#   ./run.sh survey [lib…]    correctness surveys  -> $OUT/survey_c.jsonl
#   ./run.sh bench  [lib…]    benchmarks           -> $OUT/bench_c.jsonl
#   ./run.sh resources [lib…] CPU time and peak RSS, one process per benchmark
#                                                  -> $OUT/resources_c.jsonl
#
# OUT (default ../results/<date>), COUNT (3) and BENCHTIME (seconds, 1) as in
# PROTOCOL.md. Each library executable writes $OUT/c-parts/<kind>_<lib>.jsonl
# ($OUT/c-res-parts/resources_<lib>.jsonl for resources); the parts present
# are merged after every run, so one library can be rerun alone. Needs
# CMake, Ninja, pkg-config and the Homebrew formulas listed in
# COVERAGE.md (mupdf, qpdf, libxml2, libxslt).
set -euo pipefail
cd "$(dirname "$0")"

LIBS=(mupdf qpdf-c libxml2 libxslt)

OUT=${OUT:-../results/$(date +%F)}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
export OUT COUNT=${COUNT:-3} BENCHTIME=${BENCHTIME:-1}

BUILD=build
ART=.artefacts
export FX_ARTEFACTS=$PWD/$ART
export FX_ECOSYSTEM=$(cd ../../ecosystem && pwd)

# Official artefacts (GitHub releases), checked by SHA-256. The CEN and KoSIT
# archives are the ones of the Python bench (same URLs, same sums).
KOSIT_CFG_URL=https://github.com/itplr-kosit/validator-configuration-xrechnung/releases/download/v2026-08-31/xrechnung-3.0.2-validator-configuration-2026-08-31.zip
KOSIT_CFG_SHA256=2530cd107c414511c5d0462ec10f886910395abfca820db82e83d70bf01221a8
CEN_VERSION=1.3.17
CEN_URL=https://github.com/ConnectingEurope/eInvoicing-EN16931/releases/download/validation-$CEN_VERSION/en16931-cii-$CEN_VERSION.zip
CEN_SHA256=e5f3235824b6b38454b1d54cf3f14e01efb5b33971fbefc4e64997d0ef619e89
XR_VERSION=2.6.0
XR_URL=https://github.com/itplr-kosit/xrechnung-schematron/releases/download/v$XR_VERSION/xrechnung-3.0.2-schematron-$XR_VERSION.zip
XR_SHA256=ca5e07afd04e72cd283d581590ffff3a15f4aac9d2f40c993345d78e67ea22b4

libs() { if [ $# -gt 0 ]; then echo "$@"; else echo "${LIBS[@]}"; fi; }

# fetch url sha256 file-name dest-dir: download once, check, unpack afresh.
fetch() {
	local url=$1 sum=$2 file=$ART/dl/$3 dest=$ART/$4
	mkdir -p "$ART/dl"
	if [ ! -f "$file" ] || ! echo "$sum  $file" | shasum -a 256 -c --status; then
		echo "== $url"
		curl -sSfL -o "$file.part" "$url"
		echo "$sum  $file.part" | shasum -a 256 -c --status || {
			echo "unexpected SHA-256 for $url" >&2
			rm -f "$file.part"
			exit 1
		}
		mv "$file.part" "$file"
	fi
	rm -rf "$dest" && mkdir -p "$dest" && unzip -q -o "$file" -d "$dest"
}

prefixes() {
	local p out=""
	for f in libxml2 libxslt mupdf qpdf; do
		p=$(brew --prefix "$f" 2>/dev/null) && out="$out;$p"
	done
	echo "${out#;};$(brew --prefix 2>/dev/null || echo /opt/homebrew)"
}

setup() {
	fetch "$KOSIT_CFG_URL" "$KOSIT_CFG_SHA256" "xrechnung-3.0.2-validator-configuration-2026-08-31.zip" kosit-config
	fetch "$CEN_URL" "$CEN_SHA256" "en16931-cii-$CEN_VERSION.zip" "cen-$CEN_VERSION"
	fetch "$XR_URL" "$XR_SHA256" "xrechnung-3.0.2-schematron-$XR_VERSION.zip" "xrechnung-$XR_VERSION"
	[ -d ../../ecosystem/corpus/XML-Rechnung ] || echo "corpus missing: run ../../ecosystem/run.sh setup" >&2
	cmake -S . -B "$BUILD" -G Ninja -DCMAKE_BUILD_TYPE=Release "-DCMAKE_PREFIX_PATH=$(prefixes)"
	local failed=()
	for l in $(libs "$@"); do
		echo "== build $l"
		cmake --build "$BUILD" --target "bench-$l" || failed+=("$l")
	done
	[ ${#failed[@]} -eq 0 ] || {
		echo "== not built: ${failed[*]}"
		return 1
	}
}

merge() {
	local kind=$1 parts=$OUT/${2:-c-parts} dst="$OUT/${1}_c.jsonl"
	: >"$dst"
	for l in "${LIBS[@]}"; do
		[ -f "$parts/${kind}_$l.jsonl" ] && cat "$parts/${kind}_$l.jsonl" >>"$dst"
	done
	echo "== $dst ($(wc -l <"$dst" | tr -d ' ') lines)"
}

each() {
	local kind=$1
	shift
	for l in $(libs "$@"); do
		local bin="$BUILD/bench-$l"
		if [ ! -x "$bin" ]; then
			echo "== $kind $l: $bin missing (./run.sh setup)" >&2
			continue
		fi
		"$bin" "$kind" || echo "== $kind $l: exit $?" >&2
	done
	merge "$kind"
}

# Benchmarks in the protocol's order, the order of the bench pass.
BENCHES=(Extract_Small Extract_Large Extract_Minimum Extract_Extended Parse_Small Parse_Lines Parse_Large
	ParseValidate_Small ParseValidate_Lines ParseValidate_Large Write_Small Write_Lines Embed_Small
	EmbedClassic_Small CheckPDF_Small CheckPDF_Large)

# resources: for each library and benchmark, one process, the library
# executable itself (`bench-<lib> resources <Bench>`, no wrapper), runs that
# benchmark exactly as bench does and appends its record, with
# cpu_ns_per_op and the peak RSS of the process (getrusage), to
# $OUT/c-res-parts/resources_<lib>.jsonl. A process that ends without a
# record (killed) gets a "fail" record here.
resources() {
	mkdir -p "$OUT/c-res-parts"
	for l in $(libs "$@"); do
		local bin="$BUILD/bench-$l" part="$OUT/c-res-parts/resources_$l.jsonl" b n rc
		if [ ! -x "$bin" ]; then
			echo "== resources $l: $bin missing (./run.sh setup)" >&2
			continue
		fi
		echo "== resources $l" >&2
		: >"$part"
		for b in "${BENCHES[@]}"; do
			n=$(wc -l <"$part")
			rc=0
			"$bin" resources "$b" || rc=$?
			if [ "$(wc -l <"$part")" -eq "$n" ]; then
				echo "   $b: exit $rc without a record" >&2
				printf '{"lang":"c","lib":"%s","bench":"%s","status":"fail","ns_per_op":[],"cpu_ns_per_op":[],"iters":0,"first_call_ns":0,"note":"process ended without a record (exit %s)"}\n' \
					"$l" "$b" "$rc" >>"$part"
			fi
		done
	done
	merge resources c-res-parts
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup "$@" ;;
survey) each survey "$@" ;;
bench) each bench "$@" ;;
resources) resources "$@" ;;
*)
	sed -n '2,15p' "$0"
	exit 2
	;;
esac
