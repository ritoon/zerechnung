#!/usr/bin/env bash
# Compares the rules that this package raises with the ones raised by the
# reference validators, invoice by invoice: the KoSIT validator 1.6.3 with the
# XRechnung 3.0.2 configuration of 2026-08-31, and Mustang 2.26.0; and, for
# NLCIUS, the XSLT of the schematrons of the Dutch Peppol Authority (NPa),
# run by Saxon-HE 12.10. See README.md.
#
#   conformance/parity.sh [dir]
#
# dir (default conformance/.artifacts) receives the artefacts of fetch.sh,
# the reference validators (checked by SHA-256) and the work files. The
# report is written to dir/parity/report.md and report.json, and appended to
# $GITHUB_STEP_SUMMARY under GitHub Actions. The script exits with status 1
# when a difference, or an NLCIUS assertion that no invoice makes fail, is
# not listed in parity/known.txt.
#
# Java 17 or later runs the reference validators; the package itself stays
# pure Go. Besides java, it needs what fetch.sh needs, and unzip.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

DEST=${1:-$here/.artifacts}
for tool in java unzip; do
	command -v "$tool" >/dev/null || die "$tool is required"
done
eval "$("$here/fetch.sh" "$DEST")"
DEST=$(cd "$DEST" && pwd)

TOOLS=$DEST/tools
KOSIT_JAR=$TOOLS/validator-1.6.3-standalone.jar
KOSIT_CONFIG=$FX_KOSIT_CONF # fetched and checked by fetch.sh
MUSTANG_JAR=$TOOLS/Mustang-CLI-2.26.0.jar
SAXON_JAR=$TOOLS/Saxon-HE-12.10.jar
RESOLVER_JAR=$TOOLS/xmlresolver-5.3.3.jar
RESOLVER_DATA_JAR=$TOOLS/xmlresolver-5.3.3-data.jar
fetch_file "$KOSIT_JAR" \
	https://github.com/itplr-kosit/validator/releases/download/v1.6.3/validator-1.6.3-standalone.jar \
	799e64befca97d4080e03608c80b85dd5a5ecc5f4ae4f35d1116ec2855b9a7c9
fetch_file "$MUSTANG_JAR" \
	https://github.com/ZUGFeRD/mustangproject/releases/download/core-2.26.0/Mustang-CLI-2.26.0.jar \
	42d7868cb68264874a7b8cab4c3587b03b23ccc7cd72373da917f66758bb9736
# Saxon-HE 12.10 and the XML Resolver it requires, from Maven Central, for the
# NPa XSLT. The Saxon 9.6 jar of the NPa repository stops on some XPath
# comparisons of an untyped value with a number (" 0389 " = 380).
fetch_file "$SAXON_JAR" \
	https://repo1.maven.org/maven2/net/sf/saxon/Saxon-HE/12.10/Saxon-HE-12.10.jar \
	b571af282f25d7301059f788b9a149aab8b5cdc14ef3d212dc5425d3dcbb9a97
fetch_file "$RESOLVER_JAR" \
	https://repo1.maven.org/maven2/org/xmlresolver/xmlresolver/5.3.3/xmlresolver-5.3.3.jar \
	1fe4d5b92f708dcdb82dbce12919e0171e6b5ca62c6dca6220483625098feb5f
fetch_file "$RESOLVER_DATA_JAR" \
	https://repo1.maven.org/maven2/org/xmlresolver/xmlresolver/5.3.3/xmlresolver-5.3.3-data.jar \
	b0c487ad2f3e558be8d829c916d2458d10aca6a5bafa7a4d0524b70845e48a5c
CONFIG=$TOOLS/kosit-config
rm -rf "$CONFIG"
unzip -q "$KOSIT_CONFIG" -d "$CONFIG"

WORK=$DEST/parity
# The documents of the mutation cases of the EN 16931 and XRechnung rules,
# compared with the KoSIT validator along with the corpus and the KoSIT test
# suite, one directory per rule package; and those of the Dutch rules,
# compared with the NPa XSLT along with the SimplerInvoicing test files and
# the NLCIUS invoices of the repository.
rm -rf "$WORK/variants" "$WORK/nl-variants"
for pkg in de en16931 ubl; do
	(cd "$here/.." && go test -count=1 -run '^TestVariants$' "./internal/rules/$pkg" -variants "$WORK/variants/$pkg" >/dev/null)
done
(cd "$here/.." && go test -count=1 -run '^TestVariants$' ./internal/rules/nl -variants "$WORK/nl-variants" >/dev/null)
(cd "$here/parity" && go run . stage -corpus "$FX_CORPUS" -kosit "$FX_KOSIT" -work "$WORK" -variants "$WORK/variants" \
	-nl "$FX_NL" -nl-tests "$FX_NL_TESTS" -repo "$here/.." -nl-variants "$WORK/nl-variants")
rm -rf "$WORK/kosit" "$WORK/mustang" "$WORK/npa"
mkdir -p "$WORK/kosit" "$WORK/mustang" "$WORK/npa"

# One JVM per validator. The KoSIT validator returns the number of rejected
# invoices, or a negative status (255, 254) when it cannot run.
log "KoSIT validator 1.6.3 ($(wc -l <"$WORK/kosit.list" | tr -d ' ') XML files)"
files=()
while IFS= read -r f; do
	files+=("$f")
done <"$WORK/kosit.list"
status=0
java -jar "$KOSIT_JAR" -s "$CONFIG/scenarios.xml" -r "$CONFIG" -o "$WORK/kosit" "${files[@]}" >"$WORK/kosit.log" 2>&1 || status=$?
if [ "$status" -ge 254 ]; then
	tail -20 "$WORK/kosit.log" >&2
	die "the KoSIT validator failed (status $status), see $WORK/kosit.log"
fi
log "Mustang 2.26.0 ($(wc -l <"$WORK/mustang.list" | tr -d ' ') files)"
if ! java -cp "$MUSTANG_JAR" "$here/parity/MustangBatch.java" "$WORK/mustang" "$WORK/mustang.list" >"$WORK/mustang.log" 2>&1; then
	tail -20 "$WORK/mustang.log" >&2
	die "Mustang failed, see $WORK/mustang.log"
fi
log "NPa XSLT, Saxon-HE 12.10 ($(wc -l <"$WORK/npa.list" | tr -d ' ') XML files)"
if ! java -cp "$SAXON_JAR:$RESOLVER_JAR:$RESOLVER_DATA_JAR" "$here/parity/NPaBatch.java" "$WORK/npa" "$WORK/npa.list" >"$WORK/npa.log" 2>&1; then
	tail -20 "$WORK/npa.log" >&2
	die "the NPa XSLT failed, see $WORK/npa.log"
fi

log "comparison"
status=0
(cd "$here/parity" && go run . compare -work "$WORK" -kosit-config "$CONFIG" -known "$here/parity/known.txt" -md "$WORK/report.md" -json "$WORK/report.json") || status=$?
if [ -n "${GITHUB_STEP_SUMMARY:-}" ] && [ -f "$WORK/report.md" ]; then
	{
		echo "## Parité avec KoSIT, Mustang et les XSLT de la NPa"
		echo
		cat "$WORK/report.md"
	} >>"$GITHUB_STEP_SUMMARY"
fi
exit "$status"
