#!/usr/bin/env bash
# Checks that the CAdES and XAdES signatures of package fatturapa
# interoperate with OpenSSL, xmlsec1 and EU DSS 6.5.1, both ways.
#
#   conformance/signature.sh [dir]
#
# dir defaults to conformance/.artifacts; the files go to dir/signature.
# The script:
#
#  1. writes a test PKI and the files that SignCAdES and SignXAdES produce
#     with RSA, RSA-PSS and ECDSA keys (TestWriteInteropFiles, FX_SIG_OUT);
#  2. verifies them with openssl cms -verify -cades (which requires the
#     signing-certificate-v2 attribute), xmlsec1 --verify (RSA-PSS from
#     xmlsec1 1.3 only: xmlsec1 1.2 does not know it) and DSS
#     (conformance/signature/DssCheck.java), which must find each file a
#     Baseline B signature;
#  3. signs the same document with the same keys: openssl cms -sign (with
#     and without -cades, RSA-PSS, BER streaming, PEM), xmlsec1 --sign
#     (exclusive C14N, C14N 1.1 with ECDSA, the XPath transform with
#     comments) and DSS (XAdES and CAdES Baseline B, RSA, RSA-PSS, ECDSA);
#  4. verifies those files with Unwrap and VerifySignature
#     (TestForeignSignatures, FX_SIG_FOREIGN).
#
# It needs go, openssl 3, xmlsec1, java 21 and mvn (Maven). Java and xmlsec1
# run in the CI only: the package itself stays pure Go.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

for tool in go openssl xmlsec1 java mvn; do
	command -v "$tool" >/dev/null || die "$tool is required"
done

DEST=${1:-$here/.artifacts}
mkdir -p "$DEST"
WORK=$(cd "$DEST" && pwd)/signature
rm -rf "$WORK"
OURS=$WORK/ours       # signed by the package, with the test PKI
FOREIGN=$WORK/foreign # signed by the other tools, for TestForeignSignatures
KEYS=$WORK/keys
mkdir -p "$OURS" "$FOREIGN" "$KEYS"

log "files signed by the package"
(cd "$here/.." && FX_SIG_OUT="$OURS" go test -count=1 -run '^TestWriteInteropFiles$' ./fatturapa)
mv "$OURS"/signer-*.key "$KEYS"/
cp "$OURS"/fpa12.xml "$OURS"/root.pem "$OURS"/intermediate.pem "$FOREIGN"/
doc=$OURS/fpa12.xml

log "DSS classpath"
mvn -q -B -f "$here/signature/pom.xml" dependency:build-classpath -Dmdep.outputFile="$WORK/classpath.txt" >"$WORK/maven.log" 2>&1 ||
	{ cat "$WORK/maven.log" >&2; die "Maven failed"; }
dss() { java -cp "$(cat "$WORK/classpath.txt")" "$here/signature/DssCheck.java" "$@"; }

log "openssl $(openssl version | cut -d' ' -f2) verifies the CAdES files"
for f in "$OURS"/cades-*.xml.p7m; do
	openssl cms -verify -cades -binary -inform DER -in "$f" -CAfile "$OURS/root.pem" -purpose any -out "$WORK/content.xml" 2>"$WORK/openssl.log" ||
		{ cat "$WORK/openssl.log" >&2; die "openssl: ${f##*/} does not verify"; }
	cmp -s "$WORK/content.xml" "$doc" || die "openssl: ${f##*/} does not hold fpa12.xml"
	echo "PASS ${f##*/}"
done

# xmlsec1 1.3 knows RSA-PSS and has --lax-key-search; xmlsec1 1.2 has neither.
xmlsec13=false
if xmlsec1 --help-sign 2>&1 | grep -q -- --lax-key-search; then
	xmlsec13=true
fi
log "$(xmlsec1 --version) verifies the XAdES files"
for f in "$OURS"/xades-*.xml; do
	case $f in *-pss.xml) $xmlsec13 || { echo "SKIP ${f##*/}: $(xmlsec1 --version) does not know RSA-PSS"; continue; } ;; esac
	xmlsec1 --verify --trusted-pem "$OURS/root.pem" --id-attr:Id "http://uri.etsi.org/01903/v1.3.2#:SignedProperties" \
		"$f" >"$WORK/xmlsec1.log" 2>&1 || { cat "$WORK/xmlsec1.log" >&2; die "xmlsec1: ${f##*/} does not verify"; }
	echo "PASS ${f##*/}"
done

log "DSS verifies the CAdES and XAdES files"
dss verify "$OURS/root.pem" "$OURS/intermediate.pem" "$OURS"/cades-*.xml.p7m "$OURS"/xades-*.xml

log "openssl signs"
sign_openssl() {
	local out=$1 key=$2
	shift 2
	openssl cms -sign -binary -nodetach -md sha256 -in "$doc" -signer "$OURS/signer-$key.pem" -inkey "$KEYS/signer-$key.key" \
		-certfile "$OURS/intermediate.pem" "$@" -out "$FOREIGN/$out"
	echo "wrote $out"
}
sign_openssl openssl-cades-rsa.xml.p7m rsa -cades -outform DER
sign_openssl openssl-cades-pss.xml.p7m rsa -cades -keyopt rsa_padding_mode:pss -outform PEM
sign_openssl openssl-cades-ec-stream.xml.p7m ec -cades -stream -outform DER
sign_openssl openssl-plain-rsa.xml.p7m rsa -outform DER

log "xmlsec1 signs"
# xmlsec1 1.3 looks for the signing key by the name in <ds:KeyInfo> only,
# unless --lax-key-search.
lax=()
if $xmlsec13; then
	lax=(--lax-key-search)
fi
# sign_xmlsec1 out key c14n signature-method transform: fpa12.xml with a
# signature template as the last child of the root, signed by xmlsec1.
sign_xmlsec1() {
	local out=$1 key=$2 c14n=$3 method=$4 transform=$5
	awk -v c14n="$c14n" -v method="$method" -v transform="$transform" '
		/^<\/p:FatturaElettronica>/ {
			print "<ds:Signature xmlns:ds=\"http://www.w3.org/2000/09/xmldsig#\">"
			print "<ds:SignedInfo><ds:CanonicalizationMethod Algorithm=\"" c14n "\"/>"
			print "<ds:SignatureMethod Algorithm=\"http://www.w3.org/2001/04/xmldsig-more#" method "\"/>"
			print "<ds:Reference URI=\"\"><ds:Transforms>" transform "</ds:Transforms>"
			print "<ds:DigestMethod Algorithm=\"http://www.w3.org/2001/04/xmlenc#sha256\"/><ds:DigestValue/></ds:Reference>"
			print "</ds:SignedInfo><ds:SignatureValue/>"
			print "<ds:KeyInfo><ds:X509Data><ds:X509Certificate/></ds:X509Data></ds:KeyInfo></ds:Signature>"
		}
		{ print }' "$doc" >"$WORK/template.xml"
	xmlsec1 --sign "${lax[@]}" --privkey-pem "$KEYS/signer-$key.key,$OURS/signer-$key.pem" --output "$FOREIGN/$out" "$WORK/template.xml"
	echo "wrote $out"
}
enveloped='<ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"/>'
exc=http://www.w3.org/2001/10/xml-exc-c14n#
c14n11=http://www.w3.org/2006/12/xml-c14n11
sign_xmlsec1 xmlsec1-exc-rsa.xml rsa "$exc" rsa-sha256 "$enveloped<ds:Transform Algorithm=\"$exc\"/>"
sign_xmlsec1 xmlsec1-c14n11-ec.xml ec "$c14n11" ecdsa-sha256 "$enveloped<ds:Transform Algorithm=\"$c14n11\"/>"
sign_xmlsec1 xmlsec1-xpath-rsa.xml rsa "http://www.w3.org/TR/2001/REC-xml-c14n-20010315#WithComments" rsa-sha256 \
	'<ds:Transform Algorithm="http://www.w3.org/TR/1999/REC-xpath-19991116"><ds:XPath>not(ancestor-or-self::ds:Signature)</ds:XPath></ds:Transform>'

log "DSS signs"
for key in rsa ec; do
	openssl pkcs12 -export -inkey "$KEYS/signer-$key.key" -in "$OURS/signer-$key.pem" -certfile "$OURS/intermediate.pem" \
		-passout pass:test -out "$KEYS/signer-$key.p12"
done
dss sign "$KEYS/signer-rsa.p12" test "$doc" "$FOREIGN" rsa
dss sign "$KEYS/signer-rsa.p12" test "$doc" "$FOREIGN" pss pss
dss sign "$KEYS/signer-ec.p12" test "$doc" "$FOREIGN" ec

log "the package verifies $(find "$FOREIGN" -name '*-*' | wc -l | tr -d ' ') files signed by the other tools"
(cd "$here/.." && FX_SIG_FOREIGN="$FOREIGN" go test -count=1 -v -run '^TestForeignSignatures$' ./fatturapa |
	grep -E '^(ok|FAIL|---|\s+---|\s+interop_test)')
