#!/usr/bin/env bash
# Lists the fuzz targets of the repository, one per line:
#
#   module<TAB>package<TAB>target
#
# module is the directory of the go.mod (. for package zerechnung), package
# the directory of the package relative to it (./internal/pdf), target the
# name of the function. The targets are those that go test -list finds in
# the directories where a _test.go file declares a func Fuzz: a target added
# anywhere is fuzzed by the Fuzzing workflow without any change to it.
#
# Fuzz every target for 30 seconds:
#
#   scripts/fuzz-targets.sh | while IFS=$'\t' read -r mod pkg target; do
#     (cd "$mod" && go test -run '^$' -fuzz "^$target\$" -fuzztime 30s "$pkg") || break
#   done
set -euo pipefail

cd "$(dirname "$0")/.."

git grep -l -E '^func Fuzz[A-Z0-9_]' -- '*_test.go' | xargs -n1 dirname | sort -u |
	while read -r dir; do
		mod=$dir
		while [ ! -f "$mod/go.mod" ] && [ "$mod" != . ]; do
			mod=$(dirname "$mod")
		done
		if [ "$mod" = "$dir" ]; then
			pkg=.
		elif [ "$mod" = . ]; then
			pkg=./$dir
		else
			pkg=./${dir#"$mod"/}
		fi
		(cd "$mod" && go test -list '^Fuzz' "$pkg") | grep -E '^Fuzz' |
			while read -r target; do
				printf '%s\t%s\t%s\n' "$mod" "$pkg" "$target"
			done
	done
