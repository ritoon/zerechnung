#!/usr/bin/env bash
# Runs, before a commit, the checks of the CI and Conformité workflows that
# need neither Java nor Docker, and stops at the first that fails:
#
#   scripts/check.sh          every check below
#   scripts/check.sh -quick   without the tests on the official artefacts (6)
#
#  1. gofmt on every Go file of the repository;
#  2. go vet and golangci-lint, at the version of the CI, on the package and
#     the server;
#  3. go test -race on the package and the server, and the build of the
#     command;
#  4. the allocation budgets, which skip under -race;
#  5. go vet and go test on the other modules: the site, whose examples call
#     the package, and the tools of conformance/;
#  6. the tests on the official artefacts: conformance/fetch.sh fetches them
#     at pinned versions, then go test -tags ruletrace runs every test of the
#     package, fails if one of them is skipped, and TestRuleCoverage checks
#     the rules that the tests raise.
#
# go test ./... alone skips the tests of 6, which need the artefacts (FX_*
# variables): it does not check a change to the rules, the schemas or the
# XML. The parity with the reference validators, the signature
# interoperability, veraPDF and the Docker images run in the CI only.
#
# The artefacts go to conformance/.artifacts of the main checkout, shared by
# its worktrees, or to $ZERECHNUNG_ARTIFACTS. The first fetch takes a few
# minutes; the next ones check what is there and keep it.
set -euo pipefail

quick=false
case ${1:-} in
'') ;;
-quick) quick=true ;;
*)
	sed -n '2,/^set -euo/p' "$0" | sed '$d; s/^# \{0,1\}//' >&2
	exit 2
	;;
esac

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

step() { printf '\n==> %s\n' "$*"; }
fail() {
	printf '\nFAIL: %s\n' "$*" >&2
	exit 1
}

# The version of golangci-lint that the CI runs, read from its workflow.
lint_version=$(grep -m1 -oE 'version: v[0-9]+\.[0-9]+\.[0-9]+' .github/workflows/ci.yml | cut -d' ' -f2)
golangci_lint() {
	if command -v golangci-lint >/dev/null &&
		golangci-lint version 2>/dev/null | grep -q "version ${lint_version#v} "; then
		golangci-lint "$@"
	else
		go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$lint_version" "$@"
	fi
}

step "gofmt"
files=$(gofmt -l $(git ls-files '*.go'))
[ -z "$files" ] || fail "files not formatted with gofmt:
$files"

step "go vet"
go vet ./...
(cd server && go vet ./...)

step "golangci-lint $lint_version"
golangci_lint run
(cd server && golangci_lint run --config=../.golangci.yml)

step "go test -race"
go test -race ./...
(cd server && go test -race ./...)
go build -o /dev/null ./cmd/zerechnung

step "Allocation budgets"
go test -count=1 -run 'Allocs' ./...

step "Site and conformance tools"
for mod in site conformance/parity conformance/qr; do
	(cd "$mod" && go vet ./... && go test ./...)
done

if $quick; then
	step "Tests on the official artefacts skipped (-quick)"
	exit 0
fi

step "Tests on the official artefacts"
artifacts=${ZERECHNUNG_ARTIFACTS:-}
if [ -z "$artifacts" ]; then
	common=$(cd "$(git rev-parse --git-common-dir)" && pwd)
	artifacts=$(dirname "$common")/conformance/.artifacts
fi
eval "$(conformance/fetch.sh "$artifacts")"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# Built with the tag ruletrace, each test binary appends the rules it raises
# to rules.txt, for TestRuleCoverage; -count=1 because a cached result
# appends nothing.
if ! ZERECHNUNG_RULE_TRACE=$work/rules.txt go test -tags ruletrace -count=1 -v ./... >"$work/test.log" 2>&1; then
	grep -E '^(--- FAIL|FAIL|panic)' "$work/test.log" >&2 || tail -n 50 "$work/test.log" >&2
	trap - EXIT
	fail "go test on the official artefacts, log in $work/test.log"
fi
if grep -q -- '--- SKIP' "$work/test.log"; then
	grep -A1 -- '--- SKIP' "$work/test.log" >&2
	trap - EXIT
	fail "tests skipped although every artefact is available, log in $work/test.log"
fi
go test -count=1 -run '^TestRuleCoverage$' . -rule-coverage "$work/rules.txt"

printf '\nAll checks passed.\n'
