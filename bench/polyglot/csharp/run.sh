#!/usr/bin/env bash
# C# part of the cross-language benchmark. See ../PROTOCOL.md and COVERAGE.md.
#
#   ./run.sh setup     [lib…]   official artefacts, dotnet restore (locked) + build Release, one project per library
#   ./run.sh survey    [lib…]   correctness surveys  -> $OUT/survey_csharp.jsonl
#   ./run.sh bench     [lib…]   benchmarks           -> $OUT/bench_csharp.jsonl
#   ./run.sh resources [lib…]   CPU time and peak RSS, one process per benchmark -> $OUT/resources_csharp.jsonl
#
# OUT (default ../results/<date>), COUNT (3) and BENCHTIME (seconds, 1) as in
# PROTOCOL.md. Each library program writes $OUT/csharp-parts/<kind>_<lib>.jsonl
# ($OUT/csharp-res-parts/resources_<lib>.jsonl); the parts present are merged
# after every run, so one library can be rerun alone. One process per library
# (per library and benchmark for resources), Release build, default .NET
# runtime settings.
set -euo pipefail
cd "$(dirname "$0")"

LIBS=(zugferd-csharp international-einvoicing balsoft securibox officeimo facturxdotnet pax-xrechnung en16931 antiphon saxoncs xsd pdfpig pdfsharp)

# The benchmarks in the protocol's order (Harness.Benches), one process each for resources.
BENCHES=(Extract_Small Extract_Large Extract_Minimum Extract_Extended Parse_Small Parse_Lines Parse_Large
	ParseValidate_Small ParseValidate_Lines ParseValidate_Large Write_Small Write_Lines
	Embed_Small EmbedClassic_Small CheckPDF_Small CheckPDF_Large)

# No telemetry, no first-run banner, English messages.
export DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1 DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1 DOTNET_CLI_UI_LANGUAGE=en

OUT=${OUT:-../results/$(date +%F)}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
export OUT COUNT=${COUNT:-3} BENCHTIME=${BENCHTIME:-1}
FX_ECOSYSTEM=$(cd ../../ecosystem && pwd)
ART=$(pwd)/.artefacts
export FX_ECOSYSTEM FX_ARTEFACTS=$ART

# Official validation artefacts (GitHub releases), checked by SHA-256. The CEN
# and KoSIT schematron archives are the ones of the Python bench (same hashes);
# the KoSIT configuration brings the CII D16B schemas of the XSD floor.
CEN_VERSION=1.3.17
CEN_URL=https://github.com/ConnectingEurope/eInvoicing-EN16931/releases/download/validation-$CEN_VERSION/en16931-cii-$CEN_VERSION.zip
CEN_SHA256=e5f3235824b6b38454b1d54cf3f14e01efb5b33971fbefc4e64997d0ef619e89
XR_VERSION=2.6.0
XR_URL=https://github.com/itplr-kosit/xrechnung-schematron/releases/download/v$XR_VERSION/xrechnung-3.0.2-schematron-$XR_VERSION.zip
XR_SHA256=ca5e07afd04e72cd283d581590ffff3a15f4aac9d2f40c993345d78e67ea22b4
KOSIT_VERSION=2026-08-31
KOSIT_URL=https://github.com/itplr-kosit/validator-configuration-xrechnung/releases/download/v$KOSIT_VERSION/xrechnung-3.0.2-validator-configuration-$KOSIT_VERSION.zip
KOSIT_SHA256=2530cd107c414511c5d0462ec10f886910395abfca820db82e83d70bf01221a8

libs() { if [ $# -gt 0 ]; then echo "$@"; else echo "${LIBS[@]}"; fi; }

fetch() { # url sha256 zip-name dest-dir
	local url=$1 sum=$2 zip=$ART/dl/$3 dest=$ART/$4
	mkdir -p "$ART/dl"
	if [ ! -f "$zip" ] || ! echo "$sum  $zip" | shasum -a 256 -c --status; then
		echo "== $url"
		curl -sSfL -o "$zip.part" "$url"
		echo "$sum  $zip.part" | shasum -a 256 -c --status || {
			echo "unexpected SHA-256 for $url" >&2
			rm -f "$zip.part"
			exit 1
		}
		mv "$zip.part" "$zip"
	fi
	rm -rf "$dest" && mkdir -p "$dest" && unzip -q -o "$zip" -d "$dest"
}

setup() {
	fetch "$CEN_URL" "$CEN_SHA256" "en16931-cii-$CEN_VERSION.zip" "cen-$CEN_VERSION"
	fetch "$XR_URL" "$XR_SHA256" "xrechnung-3.0.2-schematron-$XR_VERSION.zip" "xrechnung-$XR_VERSION"
	fetch "$KOSIT_URL" "$KOSIT_SHA256" "xrechnung-3.0.2-validator-configuration-$KOSIT_VERSION.zip" "kosit-config-$KOSIT_VERSION"
	[ -d ../../ecosystem/corpus/XML-Rechnung ] || echo "corpus missing: run ../../ecosystem/run.sh setup" >&2
	echo "== $(dotnet --version) (.NET SDK)"
	local failed=()
	for l in $(libs "$@"); do
		echo "== build $l"
		# Each library is restored and built on its own, so that one that does
		# not build does not stop the others. --locked-mode: the versions of
		# packages.lock.json, transitive ones included, or an error.
		if dotnet restore "libs/$l" --locked-mode --verbosity quiet &&
			dotnet build "libs/$l" -c Release --no-restore --nologo --verbosity quiet -clp:NoSummary; then
			:
		else
			failed+=("$l")
		fi
	done
	[ ${#failed[@]} -eq 0 ] || {
		echo "== not built: ${failed[*]}"
		return 1
	}
}

merge() { # kind [parts-dir]
	local kind=$1 parts=${2:-$OUT/csharp-parts} dst="$OUT/${1}_csharp.jsonl"
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
		local dll="artifacts/bin/$l/release/bench-$l.dll"
		if [ ! -f "$dll" ]; then
			echo "== $kind $l: $dll missing (./run.sh setup)" >&2
			continue
		fi
		# The muxer rather than the apphost: it finds a Homebrew or a
		# system-wide .NET without DOTNET_ROOT.
		dotnet "$dll" "$kind" || echo "== $kind $l: exit $?" >&2
	done
	merge "$kind"
}

# time(1) reports the peak RSS of the process it waits for: in bytes on macOS
# (-l, "NNN  maximum resident set size"), in KiB with GNU time (%M).
if [ "$(uname)" = Darwin ]; then
	TIME_RSS=(-l) RSS_UNIT=1
else
	TIME_RSS=(-f '%M maximum resident set size') RSS_UNIT=1024
fi

# median FIELD LINE: median of the number array FIELD of one JSON record, "-" if empty.
median() {
	LC_ALL=C awk -v f="\"$1\":[" '{
		i = index($0, f)
		s = i ? substr($0, i + length(f)) : "]"
		k = split(substr(s, 1, index(s, "]") - 1), a, ",")
		m = 0
		for (j = 1; j <= k; j++) if (a[j] ~ /^-?[0-9.]+([eE][-+]?[0-9]+)?$/) v[++m] = a[j] + 0
		for (j = 2; j <= m; j++) {
			x = v[j]
			for (p = j - 1; p > 0 && v[p] > x; p--) v[p + 1] = v[p]
			v[p + 1] = x
		}
		if (m == 0) print "-"
		else printf "%.1f\n", m % 2 ? v[(m + 1) / 2] : (v[m / 2] + v[m / 2 + 1]) / 2
		exit
	}' <<<"$2"
}

# One process per (library, benchmark), started as bench starts it (the muxer
# on the Release dll) directly under time(1), so that the peak RSS is the one
# of the process that ran this benchmark alone. The muxer hosts the runtime in
# its own process (hostfxr, hostpolicy and coreclr are loaded, not spawned);
# Homebrew's `dotnet` is a script that execs the muxer, which keeps the PID.
# The harness writes the record (with cpu_ns_per_op) to
# csharp-res-parts/run_<lib>.jsonl; max_rss_bytes is added here, except for an
# unsupported benchmark.
resources() {
	local parts=$OUT/csharp-res-parts
	mkdir -p "$parts"
	for l in $(libs "$@"); do
		local dll="artifacts/bin/$l/release/bench-$l.dll"
		if [ ! -f "$dll" ]; then
			echo "== resources $l: $dll missing (./run.sh setup)" >&2
			continue
		fi
		local dst=$parts/resources_$l.jsonl run=$parts/run_$l.jsonl tf=$parts/time_$l.txt
		: >"$dst"
		echo "== resources $l" >&2
		for b in "${BENCHES[@]}"; do
			rm -f "$run" "$tf"
			local rc=0 rss line status note id
			/usr/bin/time "${TIME_RSS[@]}" -o "$tf" dotnet "$dll" resources "$b" || rc=$?
			rss=$(awk '/maximum resident set size/ {print $1 * '"$RSS_UNIT"'; exit}' "$tf" 2>/dev/null || true)
			line=$(grep -F "\"bench\":\"$b\"" "$run" 2>/dev/null | head -1 || true)
			if [ -z "$line" ]; then
				# No record: the process died (or a library ended it).
				id=$(sed -n '1s/.*\("pkg":"[^"]*","version":"[^"]*"\).*/\1/p' "$dst")
				line="{\"lang\":\"csharp\",\"lib\":\"$l\",${id:-\"pkg\":\"\",\"version\":\"\"},\"bench\":\"$b\",\"status\":\"fail\",\"ns_per_op\":[],\"cpu_ns_per_op\":[],\"iters\":0,\"first_call_ns\":0,\"note\":\"the process exited with status $rc without writing a record\"}"
			fi
			status=$(sed -n 's/.*"status":"\([a-z]*\)".*/\1/p' <<<"$line")
			note=$(sed -n 's/.*"note":"\(.*\)"}$/\1/p' <<<"$line")
			if [ "$status" != unsupported ] && [ -n "$rss" ]; then
				line="${line%\}},\"max_rss_bytes\":$rss}"
			fi
			printf '%s\n' "$line" >>"$dst"
			if [ "$status" = ok ]; then
				printf '   %-22s %14s ns/op  %14s cpu-ns/op  %8s MiB max RSS\n' "$b" "$(median ns_per_op "$line")" \
					"$(median cpu_ns_per_op "$line")" "$(LC_ALL=C awk -v r="${rss:-0}" 'BEGIN {printf "%.1f", r / 1048576}')" >&2
			else
				printf '   %-22s %14s  %s\n' "$b" "$status" "${note:0:100}" >&2
			fi
		done
		rm -f "$run" "$tf"
	done
	merge resources "$parts"
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup "$@" ;;
survey) each survey "$@" ;;
bench) each bench "$@" ;;
resources) resources "$@" ;;
*)
	sed -n '2,14p' "$0"
	exit 2
	;;
esac
