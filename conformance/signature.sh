#!/usr/bin/env bash
# Checks that the CAdES and XAdES signatures of package fatturapa and the
# XAdES-EPES signatures of package facturae interoperate with OpenSSL,
# xmlsec1 and EU DSS 6.5.1, and the PAdES signatures of package zerechnung
# with pdfsig (Poppler) and EU DSS, both ways.
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
#     (TestForeignSignatures, FX_SIG_FOREIGN);
#  5. writes a hybrid invoice signed by Sign in PAdES B-B (RSA, RSA-PSS,
#     ECDSA, certification followed by an approval signature), B-T and
#     B-LTA, with a test PKI of its own (TestWritePAdESInteropFiles,
#     FX_PADES_OUT);
#  6. verifies them with pdfsig, which must find every ETSI.CAdES.detached
#     signature valid (it does not verify document time-stamps), and DSS,
#     which must find each signature at the PAdES level its file name gives;
#  7. signs the unsigned invoice with DSS in PAdES Baseline B (RSA, RSA-PSS,
#     ECDSA), and verifies those files with VerifySignatures
#     (TestForeignPAdES, FX_PADES_FOREIGN).
#  8. does the same for Facturae, in dir/signature/facturae: the files that
#     facturae.Sign produces are verified by xmlsec1 and by DSS
#     (conformance/signature/FacturaeCheck.java), which must find each one
#     XAdES-EPES under the signature policy of Facturae v3.1, with the role
#     emisor and ds:KeyInfo signed; xmlsec1 signs the invoice (a plain XML
#     signature, and a template with the XAdES-EPES properties) and DSS
#     signs it under the policy (EN 319 132-1 and XAdES 1.3.2, RSA, RSA-PSS,
#     ECDSA); facturae.VerifySignature verifies those files.
#
# It needs go, openssl 3, xmlsec1, pdfsig, java 21 and mvn (Maven). Java,
# xmlsec1 and pdfsig run in the CI only: the packages stay pure Go.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

for tool in go openssl xmlsec1 pdfsig java mvn; do
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

PADES=$WORK/pades                 # signed by Sign, with a test PKI of its own
PADES_FOREIGN=$WORK/pades-foreign # signed by DSS, for TestForeignPAdES
mkdir -p "$PADES" "$PADES_FOREIGN" "$KEYS/pades"

log "PDF signed by the package"
(cd "$here/.." && FX_PADES_OUT="$PADES" go test -count=1 -run '^TestWritePAdESInteropFiles$' .)
mv "$PADES"/signer-*.key "$KEYS/pades"/
cp "$PADES"/invoice.pdf "$PADES"/root.pem "$PADES"/intermediate.pem "$PADES_FOREIGN"/

log "$(pdfsig -v 2>&1 | head -1) verifies the PDF"
for f in "$PADES"/pades-*.pdf; do
	pdfsig -nocert "$f" >"$WORK/pdfsig.log" 2>&1 || true
	awk '/Signature Type:/ { type = $NF }
		/Signature Validation:/ && type == "ETSI.CAdES.detached" { n++; if ($0 !~ /Signature is Valid\./) bad = 1 }
		END { exit (n == 0 || bad) }' "$WORK/pdfsig.log" ||
		{ cat "$WORK/pdfsig.log" >&2; die "pdfsig: ${f##*/} does not verify"; }
	echo "PASS ${f##*/}"
done

log "DSS verifies the PDF"
dss verify-pades "$PADES/root.pem" "$PADES/intermediate.pem" "$PADES"/pades-*.pdf

log "DSS signs the PDF"
for key in rsa ec; do
	openssl pkcs12 -export -inkey "$KEYS/pades/signer-$key.key" -in "$PADES/signer-$key.pem" -certfile "$PADES/intermediate.pem" \
		-passout pass:test -out "$KEYS/pades/signer-$key.p12"
done
dss sign-pades "$KEYS/pades/signer-rsa.p12" test "$PADES/invoice.pdf" "$PADES_FOREIGN" rsa
dss sign-pades "$KEYS/pades/signer-rsa.p12" test "$PADES/invoice.pdf" "$PADES_FOREIGN" pss pss
dss sign-pades "$KEYS/pades/signer-ec.p12" test "$PADES/invoice.pdf" "$PADES_FOREIGN" ec

log "the package verifies $(find "$PADES_FOREIGN" -name 'dss-pades-*.pdf' | wc -l | tr -d ' ') PDF signed by DSS"
(cd "$here/.." && FX_PADES_FOREIGN="$PADES_FOREIGN" go test -count=1 -v -run '^TestForeignPAdES$' . |
	grep -E '^(ok|FAIL|---|\s+---|\s+sign_interop_test)')

# Facturae: XAdES-EPES under the signature policy of Facturae v3.1.
FWORK=$WORK/facturae
FOURS=$FWORK/ours
FFOREIGN=$FWORK/foreign
FKEYS=$KEYS/facturae
mkdir -p "$FOURS" "$FFOREIGN" "$FKEYS"
policy=http://www.facturae.es/politica_de_firma_formato_facturae/politica_de_firma_formato_facturae_v3_1.pdf

log "Facturae files signed by the package"
(cd "$here/.." && FX_SIG_OUT="$FOURS" go test -count=1 -run '^TestWriteInteropFiles$' ./facturae)
mv "$FOURS"/signer-*.key "$FKEYS"/
cp "$FOURS"/b2g.xml "$FOURS"/root.pem "$FOURS"/intermediate.pem "$FFOREIGN"/
fdoc=$FOURS/b2g.xml
facturae_dss() { java -cp "$(cat "$WORK/classpath.txt")" "$here/signature/FacturaeCheck.java" "$@"; }

log "$(xmlsec1 --version) verifies the Facturae files"
for f in "$FOURS"/facturae-*.xml; do
	case $f in *-pss.xml) $xmlsec13 || { echo "SKIP ${f##*/}: $(xmlsec1 --version) does not know RSA-PSS"; continue; } ;; esac
	xmlsec1 --verify --trusted-pem "$FOURS/root.pem" --id-attr:Id "http://uri.etsi.org/01903/v1.3.2#:SignedProperties" \
		--id-attr:Id "http://www.w3.org/2000/09/xmldsig#:KeyInfo" \
		"$f" >"$WORK/xmlsec1.log" 2>&1 || { cat "$WORK/xmlsec1.log" >&2; die "xmlsec1: ${f##*/} does not verify"; }
	echo "PASS ${f##*/}"
done

log "DSS verifies the Facturae files as XAdES-EPES"
facturae_dss verify "$FOURS/root.pem" "$FOURS/intermediate.pem" "$FOURS"/facturae-*.xml

log "xmlsec1 signs Facturae"
# sign_xmlsec1_facturae out key signature-file: b2g.xml with the signature
# template of signature-file as the last child of the root, signed by
# xmlsec1.
sign_xmlsec1_facturae() {
	local out=$1 key=$2 sig=$3
	awk -v sig="$sig" '
		/^<\/fe:Facturae>/ { while ((getline line < sig) > 0) print line }
		{ print }' "$fdoc" >"$FWORK/template.xml"
	xmlsec1 --sign "${lax[@]}" --privkey-pem "$FKEYS/signer-$key.key,$FOURS/signer-$key.pem" \
		--id-attr:Id "http://uri.etsi.org/01903/v1.3.2#:SignedProperties" --output "$FFOREIGN/$out" "$FWORK/template.xml"
	echo "wrote $out"
}
# A plain XML signature, without XAdES properties.
cat >"$FWORK/plain.xml" <<SIG
<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#">
<ds:SignedInfo><ds:CanonicalizationMethod Algorithm="http://www.w3.org/2001/10/xml-exc-c14n#"/>
<ds:SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256"/>
<ds:Reference URI=""><ds:Transforms><ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"/></ds:Transforms>
<ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/><ds:DigestValue/></ds:Reference>
</ds:SignedInfo><ds:SignatureValue/>
<ds:KeyInfo><ds:X509Data><ds:X509Certificate/></ds:X509Data></ds:KeyInfo></ds:Signature>
SIG
sign_xmlsec1_facturae xmlsec1-plain-ec.xml ec "$FWORK/plain.xml"
# A template with the XAdES-EPES properties of the policy. xmlsec1 fills
# ds:KeyInfo after it computes the digests: no reference covers it, and
# xades:SigningCertificateV2 binds the certificate.
certdigest=$(openssl x509 -in "$FOURS/signer-rsa.pem" -outform DER | openssl dgst -sha256 -binary | openssl base64 -A)
cat >"$FWORK/epes.xml" <<SIG
<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#" Id="Signature">
<ds:SignedInfo><ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315"/>
<ds:SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"/>
<ds:Reference Id="Reference-Document" URI=""><ds:Transforms><ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"/></ds:Transforms>
<ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/><ds:DigestValue/></ds:Reference>
<ds:Reference Type="http://uri.etsi.org/01903#SignedProperties" URI="#SignedProperties">
<ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/><ds:DigestValue/></ds:Reference>
</ds:SignedInfo><ds:SignatureValue/>
<ds:KeyInfo><ds:X509Data><ds:X509Certificate/></ds:X509Data></ds:KeyInfo>
<ds:Object><xades:QualifyingProperties xmlns:xades="http://uri.etsi.org/01903/v1.3.2#" Target="#Signature">
<xades:SignedProperties Id="SignedProperties"><xades:SignedSignatureProperties>
<xades:SigningTime>$(date -u +%Y-%m-%dT%H:%M:%SZ)</xades:SigningTime>
<xades:SigningCertificateV2><xades:Cert><xades:CertDigest><ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/><ds:DigestValue>$certdigest</ds:DigestValue></xades:CertDigest></xades:Cert></xades:SigningCertificateV2>
<xades:SignaturePolicyIdentifier><xades:SignaturePolicyId><xades:SigPolicyId><xades:Identifier>$policy</xades:Identifier></xades:SigPolicyId>
<xades:SigPolicyHash><ds:DigestMethod Algorithm="http://www.w3.org/2000/09/xmldsig#sha1"/><ds:DigestValue>Ohixl6upD6av8N7pEvDABhEL6hM=</ds:DigestValue></xades:SigPolicyHash>
</xades:SignaturePolicyId></xades:SignaturePolicyIdentifier>
<xades:SignerRole><xades:ClaimedRoles><xades:ClaimedRole>emisor</xades:ClaimedRole></xades:ClaimedRoles></xades:SignerRole>
</xades:SignedSignatureProperties><xades:SignedDataObjectProperties>
<xades:DataObjectFormat ObjectReference="#Reference-Document"><xades:MimeType>text/xml</xades:MimeType></xades:DataObjectFormat>
</xades:SignedDataObjectProperties></xades:SignedProperties></xades:QualifyingProperties></ds:Object></ds:Signature>
SIG
sign_xmlsec1_facturae xmlsec1-epes-rsa.xml rsa "$FWORK/epes.xml"

log "DSS signs Facturae under the policy"
for key in rsa ec; do
	openssl pkcs12 -export -inkey "$FKEYS/signer-$key.key" -in "$FOURS/signer-$key.pem" -certfile "$FOURS/intermediate.pem" \
		-passout pass:test -out "$FKEYS/signer-$key.p12"
done
facturae_dss sign "$FKEYS/signer-rsa.p12" test "$fdoc" "$FFOREIGN" rsa
facturae_dss sign "$FKEYS/signer-rsa.p12" test "$fdoc" "$FFOREIGN" rsa-v1 v1
facturae_dss sign "$FKEYS/signer-rsa.p12" test "$fdoc" "$FFOREIGN" pss pss
facturae_dss sign "$FKEYS/signer-ec.p12" test "$fdoc" "$FFOREIGN" ec
log "DSS verifies the Facturae files it signed"
facturae_dss verify "$FOURS/root.pem" "$FOURS/intermediate.pem" "$FFOREIGN"/dss-*.xml

log "the package verifies $(find "$FFOREIGN" -name '*-*' | wc -l | tr -d ' ') Facturae files signed by the other tools"
(cd "$here/.." && FX_SIG_FOREIGN="$FFOREIGN" go test -count=1 -v -run '^TestForeignSignatures$' ./facturae |
	grep -E '^(ok|FAIL|---|\s+---|\s+interop_test)')
