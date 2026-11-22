#!/usr/bin/env bash
# Rust part of the cross-language benchmark. See ../PROTOCOL.md and COVERAGE.md.
#
#   ./run.sh setup            cargo build --release, one crate per library
#   ./run.sh survey [lib…]    correctness surveys  -> $OUT/survey_rust.jsonl
#   ./run.sh bench  [lib…]    benchmarks           -> $OUT/bench_rust.jsonl
#   ./run.sh resources [lib…] CPU time and peak RSS, one process per benchmark
#                                                  -> $OUT/resources_rust.jsonl
#
# OUT (default ../results/<date>), COUNT (3) and BENCHTIME (seconds, 1) as in
# PROTOCOL.md. Each library binary writes $OUT/rust-parts/<kind>_<lib>.jsonl
# ($OUT/rust-res-parts/resources_<lib>.jsonl for resources); the parts present
# are merged after every run, so one library can be rerun alone.
set -euo pipefail
cd "$(dirname "$0")"

LIBS=(en16931 einvoice-validator erechnung-validator core-invoice zugferd-rs factur-x pdfa pdf-invoice zugferd einvoice lopdf)

OUT=${OUT:-../results/$(date +%F)}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
export OUT COUNT=${COUNT:-3} BENCHTIME=${BENCHTIME:-1}
# lopdf (zugferd-rs, pdf-invoice) parses objects with rayon: one thread, as
# the protocol asks.
export RAYON_NUM_THREADS=1

libs() { if [ $# -gt 0 ]; then echo "$@"; else echo "${LIBS[@]}"; fi; }

# zugferd-rs reads its XSD and Schematron files at run time from the crate's
# assets/schemas directory, located here from the lock file's checkout.
schemas_dir() {
	local manifest
	manifest=$(cargo metadata --format-version 1 --locked --offline 2>/dev/null |
		grep -o '"manifest_path":"[^"]*/zugferd-rs-[0-9.]*/Cargo.toml"' | head -1 | cut -d'"' -f4) || true
	[ -n "$manifest" ] && echo "$(dirname "$manifest")/assets/schemas"
}

setup() {
	cargo fetch --locked
	local failed=()
	for l in $(libs "$@"); do
		echo "== build $l"
		# Each library is built on its own, so that it gets exactly the
		# features its own dependencies ask for, and one that does not build
		# does not stop the others. --ignore-rust-version: einvoice-validator
		# 0.4.0 declares rust-version 1.99 but builds with the 1.93 toolchain.
		cargo build --release --locked --ignore-rust-version -p "bench-$l" || failed+=("$l")
	done
	[ ${#failed[@]} -eq 0 ] || { echo "== not built: ${failed[*]}"; return 1; }
}

merge() {
	local kind=$1 parts=$OUT/${2:-rust-parts} dst="$OUT/${1}_rust.jsonl"
	: >"$dst"
	for l in "${LIBS[@]}"; do
		[ -f "$parts/${kind}_$l.jsonl" ] && cat "$parts/${kind}_$l.jsonl" >>"$dst"
	done
	echo "== $dst ($(wc -l <"$dst" | tr -d ' ') lines)"
}

each() {
	local kind=$1
	shift
	if [ -z "${ZUGFERD_SCHEMAS_DIR:-}" ]; then
		ZUGFERD_SCHEMAS_DIR=$(schemas_dir) && export ZUGFERD_SCHEMAS_DIR
	fi
	for l in $(libs "$@"); do
		local bin="target/release/bench-$l"
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

# resources: for each library and benchmark, one process, the compiled binary
# itself (`target/release/bench-<lib> resources <Bench>`, never cargo run),
# runs that benchmark exactly as bench does and appends its record, with
# cpu_ns_per_op and the peak RSS of the process (getrusage), to
# $OUT/rust-res-parts/resources_<lib>.jsonl. A process that ends without a
# record (an abort, a stack overflow) gets a "fail" record here.
resources() {
	if [ -z "${ZUGFERD_SCHEMAS_DIR:-}" ]; then
		ZUGFERD_SCHEMAS_DIR=$(schemas_dir) && export ZUGFERD_SCHEMAS_DIR
	fi
	mkdir -p "$OUT/rust-res-parts"
	for l in $(libs "$@"); do
		local bin="target/release/bench-$l" part="$OUT/rust-res-parts/resources_$l.jsonl" b n rc
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
				printf '{"lang":"rust","lib":"%s","bench":"%s","status":"fail","ns_per_op":[],"cpu_ns_per_op":[],"iters":0,"first_call_ns":0,"note":"process ended without a record (exit %s)"}\n' \
					"$l" "$b" "$rc" >>"$part"
			fi
		done
	done
	merge resources rust-res-parts
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup "$@" ;;
survey) each survey "$@" ;;
bench) each bench "$@" ;;
resources) resources "$@" ;;
*)
	sed -n '2,13p' "$0"
	exit 2
	;;
esac
