#!/usr/bin/env bash
# Compares the rules that this package raises with the ones raised by the
# reference validators, invoice by invoice: the KoSIT validator 1.6.3 with the
# XRechnung 3.0.2 configuration of 2026-08-31, and Mustang 2.26.0; for
# NLCIUS, the XSLT of the schematrons of the Dutch Peppol Authority (NPa),
# run by Saxon-HE 12.10; for the CIUS-PT, the schematron of eSPap compiled
# into an XSLT by SchXslt 1.10.1 and run by Saxon-HE 12.10. See README.md.
#
#   conformance/parity.sh [dir]
#
# dir (default conformance/.artifacts) receives the artefacts of fetch.sh,
# the reference validators (checked by SHA-256) and the work files. The
# report is written to dir/parity/report.md and report.json, and appended to
# $GITHUB_STEP_SUMMARY under GitHub Actions. The script exits with status 1
# when a difference, or an NLCIUS assertion that no invoice makes fail, is
# not listed in parity/known.txt. A CIUS-PT invoice on which Saxon stops (an
# XPath error in the test of an assertion) is no difference when the package
# holds that assertion failed.
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
SCHXSLT_JAR=$TOOLS/schxslt-1.10.1.jar
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
# SchXslt 1.10.1, from Maven Central: its XSLT compile the CIUS-PT schematron
# into the XSLT that Saxon runs.
fetch_file "$SCHXSLT_JAR" \
	https://repo1.maven.org/maven2/name/dmaus/schxslt/schxslt/1.10.1/schxslt-1.10.1.jar \
	4f4f21edab7b37f96ad59ae12a344d3510f1092ac46b6d81a4efa0120b73cb58
SAXON_CP=$SAXON_JAR:$RESOLVER_JAR:$RESOLVER_DATA_JAR
CONFIG=$TOOLS/kosit-config
rm -rf "$CONFIG"
unzip -q "$KOSIT_CONFIG" -d "$CONFIG"
SCHXSLT=$TOOLS/schxslt-1.10.1
rm -rf "$SCHXSLT"
unzip -q "$SCHXSLT_JAR" 'xslt/*' -d "$SCHXSLT"
PT_SCH=$TOOLS/cius-pt-schematron
rm -rf "$PT_SCH"
unzip -q "$FX_PT_SCH" -d "$PT_SCH" # fetched and checked by fetch.sh

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
# Likewise for the CIUS-PT: the candidate mutations of the three invoices of
# internal/rules/pt/testdata (and their list, manifest.tsv), compared with
# the CIUS-PT schematron along with the examples of eSPap and those invoices.
rm -rf "$WORK/pt-variants"
mkdir -p "$WORK/pt-variants"
(cd "$here/.." && go test -count=1 -run '^TestVariants$' ./internal/rules/pt -variants "$WORK/pt-variants" >/dev/null)
(cd "$here/parity" && go run . stage -corpus "$FX_CORPUS" -kosit "$FX_KOSIT" -work "$WORK" -repo "$here/.." \
	-variants "$WORK/variants" \
	-nl "$FX_NL" -nl-tests "$FX_NL_TESTS" -nl-variants "$WORK/nl-variants" \
	-pt "$FX_PT" -pt-variants "$WORK/pt-variants")
rm -rf "$WORK/kosit" "$WORK/mustang" "$WORK/npa" "$WORK/pt"
mkdir -p "$WORK/kosit" "$WORK/mustang" "$WORK/npa" "$WORK/pt"

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
if ! java -cp "$SAXON_CP" "$here/parity/NPaBatch.java" "$WORK/npa" "$WORK/npa.list" >"$WORK/npa.log" 2>&1; then
	tail -20 "$WORK/npa.log" >&2
	die "the NPa XSLT failed, see $WORK/npa.log"
fi
# The CIUS-PT schematron compiled by SchXslt (its pipeline: inclusion of the
# abstract and concrete patterns, instantiation, compilation into an XSLT
# that writes SVRL), then run by Saxon on every CIUS-PT file. The expansion
# of the abstract patterns recurses deeper than the default thread stack of
# the JVM on Linux x64 allows (1 MB: Saxon stops on SXLM0001), hence -Xss,
# for the run as well.
log "CIUS-PT schematron 2.1.1, SchXslt 1.10.1"
if ! java -Xss8m -cp "$SAXON_CP" net.sf.saxon.Transform -xsl:"$SCHXSLT/xslt/2.0/pipeline-for-svrl.xsl" \
	-s:"$PT_SCH/urn_feap.gov.pt_CIUS-PT_2.1.1.sch" -o:"$WORK/cius-pt.xsl" >"$WORK/schxslt.log" 2>&1; then
	tail -20 "$WORK/schxslt.log" >&2
	die "SchXslt failed on the CIUS-PT schematron, see $WORK/schxslt.log"
fi
log "CIUS-PT XSLT, Saxon-HE 12.10 ($(wc -l <"$WORK/pt.list" | tr -d ' ') XML files)"
if ! java -Xss8m -cp "$SAXON_CP" "$here/parity/CiusPtBatch.java" "$WORK/cius-pt.xsl" "$WORK/pt" "$WORK/pt.list" >"$WORK/pt.log" 2>&1; then
	tail -20 "$WORK/pt.log" >&2
	die "the CIUS-PT XSLT failed, see $WORK/pt.log"
fi

log "comparison"
status=0
(cd "$here/parity" && go run . compare -work "$WORK" -kosit-config "$CONFIG" -known "$here/parity/known.txt" -md "$WORK/report.md" -json "$WORK/report.json") || status=$?
if [ -n "${GITHUB_STEP_SUMMARY:-}" ] && [ -f "$WORK/report.md" ]; then
	{
		echo "## Parité avec KoSIT, Mustang, les XSLT de la NPa et le schematron CIUS-PT"
		echo
		cat "$WORK/report.md"
	} >>"$GITHUB_STEP_SUMMARY"
fi
exit "$status"
