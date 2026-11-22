#!/usr/bin/env bash
# Fetches the official artefacts used by the optional tests of the package
# (README, section Tests) at pinned versions, and checks them: the git
# repositories by commit, the downloaded files by SHA-256.
#
#   conformance/fetch.sh [dir]
#
# dir defaults to conformance/.artifacts; what is already there and checks out
# is kept. The script prints the FX_* variables to export, so that
#
#   eval "$(conformance/fetch.sh)" && go test ./...
#
# runs every test; under GitHub Actions it also appends them to $GITHUB_ENV.
# It needs git, curl and pdfdetach (poppler-utils), which extracts the
# reference XML of the corpus PDFs (FX_CORPUS_XML) independently of this
# package. The tests also need xmllint (libxml2-utils), which checks the
# FatturaPA documents against FX_FATTURAPA_XSD.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

DEST=${1:-$here/.artifacts}
mkdir -p "$DEST"
DEST=$(cd "$DEST" && pwd)

for tool in git curl pdfdetach; do
	command -v "$tool" >/dev/null || die "$tool is required (pdfdetach comes with poppler-utils)"
done

# ZUGFeRD/corpus: PDF and XML invoices, valid and invalid (FX_CORPUS).
CORPUS_COMMIT=84f8d8f68f6c3698aa3019d701a8b82db9ba0aac
fetch_git "$DEST/corpus" https://github.com/ZUGFeRD/corpus.git "$CORPUS_COMMIT"

# KoSIT XRechnung test suite, release v2026-08-31 for XRechnung 3.0.2 (FX_KOSIT).
KOSIT_COMMIT=44c3905c69ca236aa3b94c04be1b7e9bd066ba03
fetch_git "$DEST/xrechnung-testsuite" https://github.com/itplr-kosit/xrechnung-testsuite.git "$KOSIT_COMMIT"

# Peppol BIS Billing 3.0, tag 3.0.22: schematron unit tests (FX_PEPPOL).
PEPPOL_COMMIT=38d693d71d28d813781323a6ca97324edcc4d239
fetch_git "$DEST/peppol-bis-invoice-3" https://github.com/OpenPEPPOL/peppol-bis-invoice-3.git "$PEPPOL_COMMIT"

# FNFE-MPE France_RFE, tag v1.4.0.04: CII and Factur-X examples and the CII
# D22B schema (FX_FNFE).
RFE_COMMIT=97ba0f3d4ef2abed5780e9490d5c3a2be832ab1d
fetch_git "$DEST/France_RFE" https://github.com/fnfempe/France_RFE.git "$RFE_COMMIT"

# veraPDF corpus, staging branch (CC BY 4.0): the PDF/A-2b and PDF/A-3b test
# files only (FX_VERAPDF).
VERAPDF_COMMIT=bb75f4f0073d9350dfd058c0162a367e6fadf25e
fetch_git_sparse "$DEST/verapdf-corpus" https://github.com/veraPDF/veraPDF-corpus.git "$VERAPDF_COMMIT" PDF_A-2b PDF_A-3b

# Artefacts of the Dutch rules (FX_NL, FX_NL_TESTS), pinned in fetch-nl.sh,
# which the CI workflow runs alone.
"$here/fetch-nl.sh" "$DEST"

# KoSIT validator configuration for XRechnung 3.0.2, release 2026-08-31: the
# CII D16B schema (FX_KOSIT_CONF), and the scenarios that parity.sh runs.
fetch_file "$DEST/xrechnung-3.0.2-validator-configuration-2026-08-31.zip" \
	https://github.com/itplr-kosit/validator-configuration-xrechnung/releases/download/v2026-08-31/xrechnung-3.0.2-validator-configuration-2026-08-31.zip \
	2530cd107c414511c5d0462ec10f886910395abfca820db82e83d70bf01221a8

# OASIS UBL 2.1 package (FX_UBL_ZIP); internal/schema/gen checks the same sum.
fetch_file "$DEST/UBL-2.1.zip" https://docs.oasis-open.org/ubl/os-UBL-2.1/UBL-2.1.zip \
	60b80d76394a8a2add90723ecb8e0e2e9d826775de9749df37a72d60703f86ed

# Factur-X 1.09.2 XSD (FX_XSD and FX_XSD_DIR), one directory per profile:
# BASIC, EN 16931 and EXTENDED from the FeRD ZUGFeRD 2.5.2 package, MINIMUM
# and BASIC WL from the FNFE-MPE Factur-X 1.09 package (unchanged by the
# 1.09.2 corrigendum). FeRD and FNFE-MPE hand their packages out through a
# form only, so the files come from two public copies of those packages under
# Apache-2.0, at pinned commits; each file is checked against the SHA-256 of
# the official one (internal/schema/tables_gen.go lists the same sums).
FERD=https://raw.githubusercontent.com/LandrixSoftware/validator-configuration-zugferd/8399f5459df34b2af9cae0d613870649b3f17a58/Schema/factur-x-1.09.2
FNFE=https://raw.githubusercontent.com/backoffice-plus/e-invoice-validator/38cb7fea4417cc038cb93ae3f73e74bee15b71de/configuration/factur-x/1.09/Schema/factur-x-1.09
Q=_urn_un_unece_uncefact_data_standard_QualifiedDataType_100
R=_urn_un_unece_uncefact_data_standard_ReusableAggregateBusinessInformationEntity_100
U=_urn_un_unece_uncefact_data_standard_UnqualifiedDataType_100
while read -r profile base name sum; do
	fetch_file "$DEST/xsd/$profile/$name" "$base/$name" "$sum"
done <<EOF
MINIMUM  $FNFE/0_Factur-X_1.09_MINIMUM     Factur-X_1.09_MINIMUM.xsd       dbd1e4cafb4e8c5a3e58c05649ab7009b0c8fcbc5c41b652f3b3fc96207e87f3
MINIMUM  $FNFE/0_Factur-X_1.09_MINIMUM     Factur-X_1.09_MINIMUM$Q.xsd     7830de0041e47dd035fb482d369414422768ff215a3cf668f9fa83f720cec2cc
MINIMUM  $FNFE/0_Factur-X_1.09_MINIMUM     Factur-X_1.09_MINIMUM$R.xsd     f669cd8fc2adb1ebbc983bbfe0beed56d0bdf40e5792eb2d79f7256bcc65ab3a
MINIMUM  $FNFE/0_Factur-X_1.09_MINIMUM     Factur-X_1.09_MINIMUM$U.xsd     06d4245cc75927b3cf016fae8162249c95167158ed2e512dc63d6e7e5c89fed3
BASICWL  $FNFE/1_Factur-X_1.09_BASICWL     Factur-X_1.09_BASICWL.xsd       d3b7fbffa49a3f24289f8c3eff0a7322cd0b7c8d2d3d4ca360dcc26fc7228303
BASICWL  $FNFE/1_Factur-X_1.09_BASICWL     Factur-X_1.09_BASICWL$Q.xsd     fd6958ad55567fbffb127b62b80d9cc76039a6f90315b81cba890646b824743f
BASICWL  $FNFE/1_Factur-X_1.09_BASICWL     Factur-X_1.09_BASICWL$R.xsd     5e26c4724def9aae349865d8ad35c9bf583f16decbf59376893ff79363f0c415
BASICWL  $FNFE/1_Factur-X_1.09_BASICWL     Factur-X_1.09_BASICWL$U.xsd     412f67846ce8d9af2c83e55b83a9aed8d56a0939135a38a61fbe641b3dd99814
BASIC    $FERD/2_Factur-X_1.09.2_BASIC     Factur-X_1.09.2_BASIC.xsd       808f07370fbbeaaceabbd674ee4d7d2cb358894112366989ff7d600d0432d444
BASIC    $FERD/2_Factur-X_1.09.2_BASIC     Factur-X_1.09.2_BASIC$Q.xsd     5d72dd3f0df511b2994ea41b199685f9cc538e9ce57d50c84d21988fe1455c6e
BASIC    $FERD/2_Factur-X_1.09.2_BASIC     Factur-X_1.09.2_BASIC$R.xsd     2ca9ab167afd99ae0455db1d7119007ab1de27941752dcc55e9e24bec0511c61
BASIC    $FERD/2_Factur-X_1.09.2_BASIC     Factur-X_1.09.2_BASIC$U.xsd     ad32b1828200bdb0afd24fbe92fe90880bf357c6a51a9da95cddbdf593e3e35b
EN16931  $FERD/3_Factur-X_1.09.2_EN16931   Factur-X_1.09.2_EN16931.xsd     cb98b34e59398fcd8d48f29963a3fd6f4c49af9a8307569f4a1ced634f1aa885
EN16931  $FERD/3_Factur-X_1.09.2_EN16931   Factur-X_1.09.2_EN16931$Q.xsd   4bd1753b1f078ecc50468ec90e505a63a72c6c1f2449a893f37c72a2e933fe04
EN16931  $FERD/3_Factur-X_1.09.2_EN16931   Factur-X_1.09.2_EN16931$R.xsd   d6bf8e38c3d5595c670655ccb7ee89655ae244943aaf09a31758160caabc7894
EN16931  $FERD/3_Factur-X_1.09.2_EN16931   Factur-X_1.09.2_EN16931$U.xsd   60ee4d11da40c7ec42a4da1fb77b5d7bbffcdce42afd863fcd9da9334765315c
EXTENDED $FERD/4_Factur-X_1.09.2_EXTENDED  Factur-X_1.09.2_EXTENDED.xsd    2accff44dbdc405f38fd35d789ff5e6262396940356ed362009509cd024556c6
EXTENDED $FERD/4_Factur-X_1.09.2_EXTENDED  Factur-X_1.09.2_EXTENDED$Q.xsd  ece13000d4f99c97ef09b909e9f8646d45415c46fd24d19d3c9b771c72ae2b34
EXTENDED $FERD/4_Factur-X_1.09.2_EXTENDED  Factur-X_1.09.2_EXTENDED$R.xsd  83cef3c48a6c83149733ffdced362e230e9ae482f62acdfb8c0fb3a919452fdb
EXTENDED $FERD/4_Factur-X_1.09.2_EXTENDED  Factur-X_1.09.2_EXTENDED$U.xsd  c6c9f7b3f36c821999f20d7b7fff30db64112bc7b2a8f132e3d048d859855d00
EOF

# Factur-X 1.09.2 schematrons of BASIC, EN 16931 and EXTENDED and their code
# lists (FX_FACTURX_SCH), from the same copy of the FeRD package: the code
# list assertions of internal/rules/facturx are generated from them, and
# internal/rules/facturx/gen checks the same sums (line ends normalized to
# LF; the official package and Mustang 2.26.0 have CRLF).
while read -r dir name sum; do
	fetch_file "$DEST/facturx-sch/$name" "$FERD/$dir/$name" "$sum"
done <<EOF
2_Factur-X_1.09.2_BASIC     FACTUR-X_BASIC.sch            b2a84cee0e497733bf595c435db18ba720a9f2e16f6741a972667a2f277f6bed
2_Factur-X_1.09.2_BASIC     FACTUR-X_BASIC_codedb.xml     a150b48359a72e1c1629edddb147d5133aabd03adb6ce8b3d287d0c2a6936d21
3_Factur-X_1.09.2_EN16931   FACTUR-X_EN16931.sch          ef8c8380b499d1267b2dcf1fa709b45631a77d6d5f220c6cc2ade2fdade59473
3_Factur-X_1.09.2_EN16931   FACTUR-X_EN16931_codedb.xml   70339a41d71b02bb97683f8eea04acbd764e077a88a446b32860d89c3351b62f
4_Factur-X_1.09.2_EXTENDED  FACTUR-X_EXTENDED.sch         5b9039e31f86e90f1a34925e84d3fad206efa3ca5639bb98a0cefd3661d3e36a
4_Factur-X_1.09.2_EXTENDED  FACTUR-X_EXTENDED_codedb.xml  6e23aedbba0e3b32b3b1211d145186193215f56f755201555306c42327e75fbc
EOF

# FatturaPA 1.2 examples of the Agenzia delle Entrate (FX_FATTURAPA): three
# invoices to public administrations (FPA12) and three between private
# parties (FPR12). Their terms of use do not allow copying them into the
# repository; the tests of package fatturapa check the same sums.
FPA=https://www.fatturapa.gov.it/export/documenti/fatturapa/v1.2
while read -r name sum; do
	fetch_file "$DEST/fatturapa/$name" "$FPA/$name" "$sum"
done <<EOF
IT01234567890_FPA01.xml  e26b32f39ac87824d3255f3a9d2b5f06de6cd52dfba705ad057227a5a6d8c30b
IT01234567890_FPA02.xml  872befe17bad12323d0091b664bd8341dbd758b90511873b7badc563f167f3e2
IT01234567890_FPA03.xml  3acfe0322b4cc6312f397d2d40f7e6d2bf79e734779eddf13784185db57c506b
IT01234567890_FPR01.xml  73e61f2f71ca83937fdc2b64dfa71e45f025087b44ad73d835d8c080dc70b286
IT01234567890_FPR02.xml  f75334471b7411f4dbd9c05c403e6c6c06cbeb5f4d0d832851bc84adebdc0cf4
IT01234567890_FPR03.xml  3a9001230c6a7a0d76004a7eacecb025589ee3a3369d6d995b06eda165b6f358
EOF

# FatturaPA XML schemas (FX_FATTURAPA_XSD): 1.2.4, from which
# internal/schema/fatturapa_tables_gen.go is generated, 1.2.3, in force until
# 10/01/2027, which differs from it only by the tax regime RF21, and the
# schema of xmldsig that they import, with which xmllint checks the same
# documents as the package. The schemas of the Agenzia delle Entrate are not
# copied into the repository.
while read -r name sum url; do
	fetch_file "$DEST/fatturapa-xsd/$name" "$url" "$sum"
done <<EOF
Schema_VFPA12_V1.2.3.xsd  152944f6eef9f5d69ef6e955ee173b32142b00a8c1c5222fc97dfab5910e8a8c  https://www.fatturapa.gov.it/export/documenti/fatturapa/v1.4/Schema_VFPA12_V1.2.3.xsd
Schema_VFPA12_v1.2.4.xsd  29728f77e2739e150131258279218936287630e95566cab360cf66d53fc8f338  https://www.fatturapa.gov.it/export/documenti/fatturapa/v1.5/Schema_VFPA12_v1.2.4.xsd
xmldsig-core-schema.xsd   35cf8197da812c85e40d57891b35c94187569ed474a2dac813ce5090dafcd35c  https://www.w3.org/TR/2002/REC-xmldsig-core-20020212/xmldsig-core-schema.xsd
EOF

# Facturae 3.2, 3.2.1 and 3.2.2 XML schemas of facturae.gob.es
# (FX_FACTURAE_XSD); internal/schema/gen checks the same sums.
FACTURAE=https://www.facturae.gob.es/content/dam/facturae/formato/versiones
while read -r name sum; do
	fetch_file "$DEST/facturae-xsd/$name" "$FACTURAE/$name" "$sum"
done <<EOF
Facturaev3_2.xml    05f390ac7e2fd36da6567185cd0e420764e304b6ba491af51a8a100b07283396
Facturaev3_2_1.xml  f0f24f27729db91fdd90b715a5dc16f2d779612b4e9e9f193c2903f0e67dc6a6
Facturaev3_2_2.xml  b4bbcd587f5fb0a8a906336cca09b0a40d06ffaa78c6a62f6e438c4e6ea86e07
EOF

# CIUS-PT 2.1.1 of eSPap, page Normas of www.espap.gov.pt: the examples of
# invoices and credit notes (FX_PT), not copied into the repository as their
# terms of use are not stated, and the schematron archive (FX_PT_SCH) that
# internal/rules/pt/gen reads, which checks the same sum.
ESPAP=https://www.espap.gov.pt/Imagens/Documento.ashx
fetch_file "$DEST/cius-pt/urn_feap.gov.pt_cius-pt_examples.zip" "$ESPAP?id=254" \
	c7380610bf80e3fdaf8543846f96ffade2150432f1ac12df2041e3b2d8ed51b0
fetch_file "$DEST/cius-pt/urn_feap.gov.pt_cius-pt_schematron.zip" "$ESPAP?id=267" \
	89e908d2290d361a3ed9fc5c3dd5c7a661da249d4a3c4515b53a8f4b8cb6d005

# CIUS-RO of ANAF: the schematron 1.0.9 (FX_RO_SCH) that internal/rules/ro/gen
# reads, which checks the same sum, and the examples of invoices and credit
# notes in UBL and CII (FX_RO, the directory). mfinante.gov.ro refuses
# automated downloads: they are the copies of the archives of ANAF kept by
# phive-rules (phax), at a pinned commit.
PHIVE=https://raw.githubusercontent.com/phax/phive-rules/07840eb381ea43b0f3f8c9f346c826186afcdc4b/phive-rules-cius-ro/docs
while read -r path sum; do
	fetch_file "$DEST/cius-ro/${path##*/}" "$PHIVE/$path" "$sum"
done <<EOF
1.0.9/ro16931-ubl-1.0.9.zip          818e7dd77e429f5dbde7c60b43dddbdf1a9a82a1773c359caa80a36ef89ea7f2
eFactura_UBL2.1_exempleXML.zip       7b21c46122a3b9963167904f7fbe9f5e06b9de68d7efa7d245b1fb05f2de68a1
exemple_Invoice_CreditNote.zip       3c1ddcfc8be8ca3ee7f70d9996f24ede401b3ae4e11124221dd23d369c991c4c
eFactura_CII_exempleXML.zip          b74f662264397f9a9afab68553866a51a7bb9163333e50113e67cf1982381727
EOF

# Reference XML of the valid PDFs of the corpus (FX_CORPUS_XML), extracted by
# pdfdetach: the attachment named factur-x.xml, xrechnung.xml or
# zugferd-invoice.xml (any case), else the only XML attachment. Each file has
# the relative path of its PDF, with .xml for .pdf. Rebuilt when the corpus
# commit changes.
XML=$DEST/corpus-xml
if [ "$(cat "$XML/.commit" 2>/dev/null)" != "$CORPUS_COMMIT" ]; then
	log "pdfdetach: corpus PDFs → corpus-xml"
	rm -rf "$XML"
	mkdir -p "$XML"
	(cd "$DEST/corpus" && find ZUGFeRDv1/correct ZUGFeRDv2/correct XML-Rechnung/FX -type f -iname '*.pdf' | LC_ALL=C sort) |
		while IFS= read -r rel; do
			pdf=$DEST/corpus/$rel
			list=$(pdfdetach -list "$pdf" 2>/dev/null | sed -n 's/^\([0-9][0-9]*\): //p') || continue
			n=$(printf '%s\n' "$list" | grep -n -i -x -E 'factur-x\.xml|xrechnung\.xml|zugferd-invoice\.xml' | head -1 | cut -d: -f1 || true)
			if [ -z "$n" ] && [ "$(printf '%s\n' "$list" | grep -c -i '\.xml$' || true)" = 1 ]; then
				n=$(printf '%s\n' "$list" | grep -n -i '\.xml$' | cut -d: -f1)
			fi
			[ -n "$n" ] || continue
			out=$XML/${rel%.*}.xml
			mkdir -p "$(dirname "$out")"
			pdfdetach -save "$n" -o "$out" "$pdf" 2>/dev/null || rm -f "$out"
		done
	echo "$CORPUS_COMMIT" >"$XML/.commit"
fi

vars=(
	"FX_CORPUS=$DEST/corpus"
	"FX_CORPUS_XML=$XML"
	"FX_KOSIT=$DEST/xrechnung-testsuite"
	"FX_KOSIT_CONF=$DEST/xrechnung-3.0.2-validator-configuration-2026-08-31.zip"
	"FX_FNFE=$DEST/France_RFE"
	"FX_PEPPOL=$DEST/peppol-bis-invoice-3"
	"FX_UBL_ZIP=$DEST/UBL-2.1.zip"
	"FX_XSD=$DEST/xsd"
	"FX_XSD_DIR=$DEST/xsd"
	"FX_FACTURX_SCH=$DEST/facturx-sch"
	"FX_VERAPDF=$DEST/verapdf-corpus"
	"FX_FATTURAPA=$DEST/fatturapa"
	"FX_FATTURAPA_XSD=$DEST/fatturapa-xsd"
	"FX_FACTURAE_XSD=$DEST/facturae-xsd"
	"FX_PT=$DEST/cius-pt/urn_feap.gov.pt_cius-pt_examples.zip"
	"FX_PT_SCH=$DEST/cius-pt/urn_feap.gov.pt_cius-pt_schematron.zip"
	"FX_RO=$DEST/cius-ro"
	"FX_RO_SCH=$DEST/cius-ro/ro16931-ubl-1.0.9.zip"
)
export_vars "${vars[@]}"
