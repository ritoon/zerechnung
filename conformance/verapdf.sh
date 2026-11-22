#!/usr/bin/env bash
# Checks the files that ConvertPDFA writes with veraPDF: the PDF/A-2b and
# PDF/A-3b test files of the veraPDF corpus, converted, and the non-
# conforming PDF files that the tests of convert_test.go generate and
# convert; and the PDF/A-3 hybrid invoices that TestSignPDFA signs (PAdES
# B-B, B-T, B-LTA, certification), for the signature rules of 6.4.3.
#
#   conformance/verapdf.sh [dir]
#
# dir defaults to conformance/.artifacts. The script fetches the veraPDF
# corpus (FX_VERAPDF) at its pinned commit, converts the files with
#
#   go test -run '^(TestConvertCorpus|TestConvertPDFA|TestEmbedConvertPDFA|TestSignPDFA)$'
#
# and FX_PDFA_OUT, runs veraPDF on them, then checks the report with the
# same tests and FX_PDFA_REPORT: every converted file passes, but those that
# convert_corpus_test.go lists as known failures. veraPDF runs from its
# Docker image, pinned by digest: the package itself stays pure Go.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
. "$here/lib.sh"

# veraPDF 1.30.3 (greenfield parser), image of the veraPDF consortium.
VERAPDF_IMAGE=verapdf/cli:v1.30.3@sha256:d74b7026c18a0f171e73c153b617fe6520e1511feb24ca98a348b2bb897669fb
# veraPDF corpus, staging branch (CC BY 4.0), as in fetch.sh.
VERAPDF_COMMIT=bb75f4f0073d9350dfd058c0162a367e6fadf25e

DEST=${1:-$here/.artifacts}
mkdir -p "$DEST"
DEST=$(cd "$DEST" && pwd)

for tool in git go docker; do
	command -v "$tool" >/dev/null || die "$tool is required"
done

fetch_git_sparse "$DEST/verapdf-corpus" https://github.com/veraPDF/veraPDF-corpus.git "$VERAPDF_COMMIT" PDF_A-2b PDF_A-3b

WORK=$DEST/pdfa
rm -rf "$WORK"
mkdir -p "$WORK/out"
tests='^(TestConvertCorpus|TestConvertPDFA|TestEmbedConvertPDFA|TestSignPDFA)$'

log "conversion"
(cd "$here/.." && FX_VERAPDF="$DEST/verapdf-corpus" FX_PDFA_OUT="$WORK/out" go test -count=1 -run "$tests" .)

log "veraPDF on $(find "$WORK/out" -name '*.pdf' | wc -l | tr -d ' ') files"
docker run --rm -v "$WORK/out":/data:ro "$VERAPDF_IMAGE" --format text --recurse /data >"$WORK/report.txt" 2>"$WORK/verapdf.log" ||
	true # veraPDF exits with 1 when a file fails: the report says which
grep -c '^PASS' "$WORK/report.txt" | sed 's/^/PASS: /' >&2
grep -c '^FAIL' "$WORK/report.txt" | sed 's/^/FAIL: /' >&2 || true

log "report check"
(cd "$here/.." && FX_VERAPDF="$DEST/verapdf-corpus" FX_PDFA_OUT="$WORK/out" FX_PDFA_REPORT="$WORK/report.txt" \
	go test -count=1 -v -run "$tests" . | grep -E '^(ok|FAIL|---|\s+convert)')
