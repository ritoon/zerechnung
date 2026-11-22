#!/usr/bin/env bash
# Java part of the cross-language benchmark. See ../PROTOCOL.md and COVERAGE.md.
#
#   ./run.sh setup              official artefacts + mvn package (one module per library)
#   ./run.sh survey    [lib…]   correctness surveys  -> $OUT/survey_java.jsonl
#   ./run.sh bench     [lib…]   benchmarks           -> $OUT/bench_java.jsonl
#   ./run.sh resources [lib…]   CPU time and peak RSS, one JVM per benchmark -> $OUT/resources_java.jsonl
#
# OUT (default ../results/<date>), COUNT (3) and BENCHTIME (seconds, 1) as in
# PROTOCOL.md. JAVA_HOME selects the JDK (default: the Homebrew openjdk, since
# /usr/bin/java on macOS is only a stub). Each library runs in its own JVM with
# the JVM's default options and writes $OUT/java-parts/<kind>_<lib>.jsonl
# ($OUT/java-res-parts/resources_<lib>.jsonl); the parts present are merged
# after every run, so one library can be rerun alone.
set -euo pipefail
cd "$(dirname "$0")"

LIBS=(mustang phive kosit-validator saxon-schematron paniolo ph-cii invoice4j imixs-e-invoice gallop pdfbox)

# The benchmarks in the protocol's order (Harness.Op), one JVM each for resources.
BENCHES=(Extract_Small Extract_Large Extract_Minimum Extract_Extended Parse_Small Parse_Lines Parse_Large
	ParseValidate_Small ParseValidate_Lines ParseValidate_Large Write_Small Write_Lines
	Embed_Small EmbedClassic_Small CheckPDF_Small CheckPDF_Large)

# Official validation artefacts (GitHub releases), checked by SHA-256, used by
# the saxon-schematron floor (CEN, KoSIT schematron) and by kosit-validator
# (its configuration). The other libraries bundle their own copies in their jars.
ART=.artefacts
CEN_VERSION=1.3.17
CEN_URL=https://github.com/ConnectingEurope/eInvoicing-EN16931/releases/download/validation-$CEN_VERSION/en16931-cii-$CEN_VERSION.zip
CEN_SHA256=e5f3235824b6b38454b1d54cf3f14e01efb5b33971fbefc4e64997d0ef619e89
XR_VERSION=2.6.0
XR_URL=https://github.com/itplr-kosit/xrechnung-schematron/releases/download/v$XR_VERSION/xrechnung-3.0.2-schematron-$XR_VERSION.zip
XR_SHA256=ca5e07afd04e72cd283d581590ffff3a15f4aac9d2f40c993345d78e67ea22b4
KOSIT_CFG=2026-08-31
KOSIT_URL=https://github.com/itplr-kosit/validator-configuration-xrechnung/releases/download/v$KOSIT_CFG/xrechnung-3.0.2-validator-configuration-$KOSIT_CFG.zip
KOSIT_SHA256=2530cd107c414511c5d0462ec10f886910395abfca820db82e83d70bf01221a8

if [ -z "${JAVA_HOME:-}" ]; then
	for d in /opt/homebrew/opt/openjdk /usr/local/opt/openjdk; do
		if [ -x "$d/bin/java" ]; then
			JAVA_HOME=$d
			break
		fi
	done
fi
[ -n "${JAVA_HOME:-}" ] && [ -x "$JAVA_HOME/bin/java" ] || { echo "no JDK found: set JAVA_HOME" >&2; exit 1; }
export JAVA_HOME
JAVA=$JAVA_HOME/bin/java

OUT=${OUT:-../results/$(date +%F)}
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
export OUT COUNT=${COUNT:-3} BENCHTIME=${BENCHTIME:-1}
export FX_ECOSYSTEM=${FX_ECOSYSTEM:-$(cd ../../ecosystem && pwd)}
export FX_ARTEFACTS=${FX_ARTEFACTS:-$PWD/$ART}

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
	fetch "$KOSIT_URL" "$KOSIT_SHA256" "xrechnung-3.0.2-validator-configuration-$KOSIT_CFG.zip" \
		"kosit-xrechnung-3.0.2-$KOSIT_CFG"
	# -C: fail on a checksum mismatch of a downloaded artifact. The package
	# phase also writes each module's runtime classpath to target/classpath.txt.
	mvn -B -C -q package
	[ -d "$FX_ECOSYSTEM/corpus/XML-Rechnung" ] || echo "corpus missing: run ../../ecosystem/run.sh setup" >&2
	[ -f "$FX_ECOSYSTEM/host.pdf" ] || echo "host.pdf missing: run ../../ecosystem/run.sh setup" >&2
	echo "== $("$JAVA" -version 2>&1 | head -1), $(mvn -v 2>/dev/null | head -1)"
}

merge() { # kind [parts-dir]
	local kind=$1 parts=${2:-$OUT/java-parts} dst="$OUT/${1}_java.jsonl"
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
		local dir=libs/$l
		if [ ! -f "$dir/target/classpath.txt" ] || [ ! -d "$dir/target/classes" ]; then
			echo "== $kind $l: $dir/target missing (./run.sh setup)" >&2
			continue
		fi
		# One JVM per library, default JVM options; the harness runs the
		# operations on one thread with a large stack (no -Xss needed).
		"$JAVA" -cp "$dir/target/classes:$(cat "$dir/target/classpath.txt")" bench.Main "$kind" ||
			echo "== $kind $l: exit $?" >&2
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

# One JVM per (library, benchmark): the same command line and default JVM
# options as bench, run directly under time(1) (no wrapper in between), so that
# the peak RSS is the one of the JVM that ran this benchmark alone. The
# harness writes the record (with cpu_ns_per_op) to java-res-parts/run_<lib>.jsonl;
# max_rss_bytes is added here, except for an unsupported benchmark.
resources() {
	local parts=$OUT/java-res-parts
	mkdir -p "$parts"
	for l in $(libs "$@"); do
		local dir=libs/$l
		if [ ! -f "$dir/target/classpath.txt" ] || [ ! -d "$dir/target/classes" ]; then
			echo "== resources $l: $dir/target missing (./run.sh setup)" >&2
			continue
		fi
		local cp="$dir/target/classes:$(cat "$dir/target/classpath.txt")"
		local dst=$parts/resources_$l.jsonl run=$parts/run_$l.jsonl tf=$parts/time_$l.txt
		: >"$dst"
		echo "== resources $l" >&2
		for b in "${BENCHES[@]}"; do
			rm -f "$run" "$tf"
			local rc=0 rss line status note id
			/usr/bin/time "${TIME_RSS[@]}" -o "$tf" "$JAVA" -cp "$cp" bench.Main resources "$b" || rc=$?
			rss=$(awk '/maximum resident set size/ {print $1 * '"$RSS_UNIT"'; exit}' "$tf" 2>/dev/null || true)
			line=$(grep -F "\"bench\":\"$b\"" "$run" 2>/dev/null | head -1 || true)
			if [ -z "$line" ]; then
				# No record: the JVM died (or a library called System.exit).
				id=$(sed -n '1s/.*\("pkg":"[^"]*","version":"[^"]*"\).*/\1/p' "$dst")
				line="{\"lang\":\"java\",\"lib\":\"$l\",${id:-\"pkg\":\"\",\"version\":\"\"},\"bench\":\"$b\",\"status\":\"fail\",\"ns_per_op\":[],\"cpu_ns_per_op\":[],\"iters\":0,\"first_call_ns\":0,\"note\":\"the JVM exited with status $rc without writing a record\"}"
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
setup) setup ;;
survey) each survey "$@" ;;
bench) each bench "$@" ;;
resources) resources "$@" ;;
*)
	sed -n '2,14p' "$0"
	exit 2
	;;
esac
