#!/bin/sh
# Plenilune fuzz entry point (plan P1-6). One documented way to run every
# fuzz target, locally or in CI:
#
#   scripts/fuzz.sh seeds            seed corpora only, as ordinary tests
#   scripts/fuzz.sh short [seconds]  bounded fuzz pass per target (default 60)
#   scripts/fuzz.sh long  [seconds]  nightly-length pass per target (default 600)
#
# Local development needs nothing hosted: this script plus `go test ./...`
# is the whole gate. The GitHub Actions workflows in .github/workflows/
# only call this script (short on changes, long on a nightly schedule),
# exactly as plan item P1-6 prescribes.
#
# A fuzz failure writes the crasher under the package's testdata/fuzz/
# directory. Per the plan, a crasher becomes a regression seed/test first;
# never delete it or weaken the target to make the run green.
set -eu
cd "$(dirname "$0")/.."

mode=${1:-seeds}
seconds=${2:-}

targets="
plenilune/internal/ciscoacl:FuzzParse
plenilune/internal/ciscoacl:FuzzACLJSONRoundTrip
plenilune/internal/prefixlist:FuzzParsePrefixList
plenilune/internal/prefixlist:FuzzPrefixListRoundTrip
plenilune/internal/jsonio:FuzzDecode
plenilune/internal/packetset:FuzzAlgebraInvariants
plenilune/internal/intent:FuzzNormalizeJSON
plenilune/internal/verify:FuzzEvaluateVsRegionBruteForce
plenilune/internal/verify:FuzzCompareSmallPairs
"

case "$mode" in
seeds)
	go test ./internal/ciscoacl/ ./internal/prefixlist/ ./internal/jsonio/ \
		./internal/packetset/ ./internal/intent/ ./internal/verify/ \
		-run 'Fuzz' -count=1
	;;
short | long)
	if [ -z "$seconds" ]; then
		if [ "$mode" = short ]; then seconds=60; else seconds=600; fi
	fi
	for entry in $targets; do
		pkg=${entry%%:*}
		name=${entry##*:}
		echo "== fuzz $name ($pkg) for ${seconds}s"
		go test "$pkg" -run '^$' -fuzz "^${name}$" -fuzztime "${seconds}s" -count=1
	done
	;;
*)
	echo "usage: scripts/fuzz.sh [seeds|short [seconds]|long [seconds]]" >&2
	exit 2
	;;
esac
