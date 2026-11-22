#!/usr/bin/env bash
# PHP bench of the Factur-X / ZUGFeRD / EN 16931 libraries. See ../PROTOCOL.md and COVERAGE.md.
#
#   ./run.sh setup               composer install of the two pinned environments (vendor/, invoicesuite/vendor/)
#   ./run.sh survey    [lib…]    accuracy surveys       -> $OUT/survey_php.jsonl
#   ./run.sh bench     [lib…]    benchmarks             -> $OUT/bench_php.jsonl
#   ./run.sh resources [lib…]    CPU time and peak RSS  -> $OUT/resources_php.jsonl
#
# Variables: OUT (../results/<date>), COUNT (3), BENCHTIME (1, seconds), PHP (php), COMPOSER (composer),
# PHP_OPTS (php -d flags of every measuring process, default below), FX_TMP (scratch dir for path-based APIs).
set -euo pipefail
cd "$(dirname "$0")"

PHP=${PHP:-php}
COMPOSER=${COMPOSER:-composer}

export OUT=${OUT:-../results/$(date +%F)}
export COUNT=${COUNT:-3}
export BENCHTIME=${BENCHTIME:-1}
# The CLI as installed (OPcache off: opcache.enable_cli=0 is the CLI default, so no JIT either),
# with explicit overrides that do not change the generated code:
#   memory_limit=4G      instead of the default 128M, so that no library fails for lack of memory
#                        on the large samples or during the surveys;
#   zend.assertions=-1   production setting (the php.ini-development value 1 runs assert() calls);
#   display_errors, error_reporting
#                        notices on stderr, E_ALL without deprecations, which several libraries
#                        trigger on PHP 8.6 (printing them would only slow the run down).
export PHP_OPTS=${PHP_OPTS:-"-d memory_limit=4G -d zend.assertions=-1 -d display_errors=stderr -d error_reporting=6143"}
PHP_BIN=$(command -v "$PHP")

# Composer without scripts nor plugins: third-party code runs only when the bench calls it.
# --ignore-platform-req=php+ ignores upper bounds only (two packages declare php <8.6).
COMPOSER_FLAGS=(install --no-interaction --no-progress --no-scripts --no-plugins
	--ignore-platform-req=php+ --optimize-autoloader)

setup() {
	echo "== $("$PHP" -r 'echo "PHP ", PHP_VERSION;'), $("$COMPOSER" --version 2>/dev/null | head -1)"
	# extensions required by the pinned packages (composer.lock) and by the harness (pcntl)
	for ext in dom libxml xml simplexml xmlreader xmlwriter pcntl mbstring gd zlib bcmath fileinfo zip iconv ctype; do
		"$PHP" -r "exit(extension_loaded('$ext') ? 0 : 1);" || echo "missing PHP extension: $ext" >&2
	done
	command -v pdfdetach >/dev/null || echo "pdfdetach (poppler) not found: mahdiabderraouf and schuppelius extraction will fail" >&2
	echo "== composer install (main environment)"
	"$COMPOSER" "${COMPOSER_FLAGS[@]}" --working-dir=.
	echo "== composer install (horstoeko/invoicesuite, conflicts with horstoeko/zugferd)"
	"$COMPOSER" "${COMPOSER_FLAGS[@]}" --working-dir=invoicesuite
	[ -d ../../ecosystem/corpus/XML-Rechnung ] || echo "corpus missing: run ../../ecosystem/run.sh setup" >&2
	[ -f ../../ecosystem/host.pdf ] || echo "host.pdf missing: run ../../ecosystem/run.sh setup" >&2
	echo "== ready"
}

need() {
	[ -f vendor/autoload.php ] && [ -f invoicesuite/vendor/autoload.php ] || setup
}

cmd=${1:-}
shift || true
case "$cmd" in
setup) setup ;;
survey | bench | resources)
	need
	# resources: harness.php starts one child per benchmark, PHP_BINARY with
	# the same PHP_OPTS (no wrapper); the child reads its own CPU time and peak RSS.
	# shellcheck disable=SC2086 # PHP_OPTS is a list of flags
	"$PHP_BIN" $PHP_OPTS harness.php "$cmd" "$@"
	;;
*)
	sed -n '2,10p' "$0"
	exit 2
	;;
esac
