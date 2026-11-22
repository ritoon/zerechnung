#!/usr/bin/env bash
# Banc Python des librairies Factur-X / ZUGFeRD / EN 16931. Voir ../PROTOCOL.md et COVERAGE.md.
#
#   ./run.sh setup               venv (CPython 3.13), dépendances figées, artefacts XSLT officiels
#   ./run.sh survey    [lib…]    relevés d'exactitude  -> $OUT/survey_python.jsonl
#   ./run.sh bench     [lib…]    benchmarks            -> $OUT/bench_python.jsonl
#   ./run.sh resources [lib…]    CPU et pic mémoire    -> $OUT/resources_python.jsonl
#
# Variables : OUT (../results/<date>), COUNT (3), BENCHTIME (1, secondes), PYTHON (python3.13).
set -euo pipefail
cd "$(dirname "$0")"

PYTHON=${PYTHON:-python3.13}
VENV=.venv
ART=.artefacts

# Artefacts de validation officiels (releases GitHub), vérifiés par SHA-256.
CEN_VERSION=1.3.17
CEN_URL=https://github.com/ConnectingEurope/eInvoicing-EN16931/releases/download/validation-$CEN_VERSION/en16931-cii-$CEN_VERSION.zip
CEN_SHA256=e5f3235824b6b38454b1d54cf3f14e01efb5b33971fbefc4e64997d0ef619e89
XR_VERSION=2.6.0
XR_URL=https://github.com/itplr-kosit/xrechnung-schematron/releases/download/v$XR_VERSION/xrechnung-3.0.2-schematron-$XR_VERSION.zip
XR_SHA256=ca5e07afd04e72cd283d581590ffff3a15f4aac9d2f40c993345d78e67ea22b4

export OUT=${OUT:-../results/$(date +%F)}
export COUNT=${COUNT:-3}
export BENCHTIME=${BENCHTIME:-1}

fetch() { # url sha256 zip-name dest-dir
	local url=$1 sum=$2 zip=$ART/dl/$3 dest=$ART/$4
	mkdir -p "$ART/dl"
	if [ ! -f "$zip" ] || ! echo "$sum  $zip" | shasum -a 256 -c --status; then
		echo "== $url"
		curl -sSfL -o "$zip.part" "$url"
		echo "$sum  $zip.part" | shasum -a 256 -c --status || { echo "SHA-256 inattendu pour $url" >&2; rm -f "$zip.part"; exit 1; }
		mv "$zip.part" "$zip"
	fi
	rm -rf "$dest" && mkdir -p "$dest" && unzip -q -o "$zip" -d "$dest"
}

setup() {
	if [ ! -x "$VENV/bin/python" ]; then
		echo "== venv ($($PYTHON --version))"
		"$PYTHON" -m venv "$VENV"
	fi
	"$VENV/bin/python" -m pip install --quiet --disable-pip-version-check \
		--only-binary=:all: --no-binary=tagic -r requirements.txt
	"$VENV/bin/python" -m pip check
	fetch "$CEN_URL" "$CEN_SHA256" "en16931-cii-$CEN_VERSION.zip" "cen-$CEN_VERSION"
	fetch "$XR_URL" "$XR_SHA256" "xrechnung-3.0.2-schematron-$XR_VERSION.zip" "xrechnung-$XR_VERSION"
	[ -d ../../ecosystem/corpus/XML-Rechnung ] || echo "corpus absent : lancer ../../ecosystem/run.sh setup" >&2
	[ -f ../../ecosystem/host.pdf ] || echo "host.pdf absent : lancer ../../ecosystem/run.sh setup" >&2
	echo "== $("$VENV/bin/python" --version), artefacts CEN $CEN_VERSION et KoSIT XRechnung $XR_VERSION prêts"
}

need() {
	[ -x "$VENV/bin/python" ] && [ -d "$ART/cen-$CEN_VERSION" ] || setup
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup ;;
survey | bench | resources)
	need
	# resources: harness.py starts one child per benchmark with the same
	# interpreter (sys.executable, the venv's python, no wrapper); the child
	# reads its own CPU time and peak RSS.
	"$VENV/bin/python" -I harness.py "$cmd" "$@"
	;;
*)
	sed -n '2,9p' "$0"
	exit 2
	;;
esac
