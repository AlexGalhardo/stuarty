#!/usr/bin/env bash
# Runs every fuzz target for a fixed time budget (SDD-008 section 13.2):
#   bash scripts/fuzz.sh [budget, default 30s] [report file]
# Go runs one target of one package at a time, so each line of the list below is its own run. A crash is
# minimised by the fuzzer into testdata/fuzz/<Target>/ of the package and stays there as a regression case that
# `go test` runs from then on. The script prints one line per target (target, package, seconds, result), writes
# the same lines to the report when one is given, and exits 1 when any target failed. The workers are 4 by default
# (FUZZ_PARALLEL): with one per core the machine is so loaded that the targets that assert a time (FuzzFontHeader)
# fail for the load, not for the input.
set -uo pipefail

budget="${1:-30s}"
report="${2:-}"

# <package> <target>
targets=(
	"./core/command FuzzApplyBatch"
	"./core/command FuzzApplyArgs"
	"./core/image FuzzApplyArgs"
	"./core/image FuzzDecode"
	"./core/timeline FuzzDecode"
	"./core/timeval FuzzParse"
	"./core/subtitle FuzzParse"
	"./core/otio FuzzImport"
	"./core/library FuzzParseFrontMatter"
	"./core/library FuzzParseSkill"
	"./engine/ffmpeg FuzzSniffMedia"
	"./engine/ffmpeg FuzzFFArgsCheck"
	"./engine/ingest FuzzChunkIndex"
	"./engine/lut FuzzCubeParse"
	"./engine/rasterize FuzzFontHeader"
	"./engine/renderhost FuzzRenderInput"
	"./service/pathguard FuzzCanonicalizeImportPath"
	"./service FuzzTemplateArchive"
	"./service/untrusted FuzzUntrustedClean"
	"./service/connections/oauth FuzzParseEnvelope"
	"./service/publish/destinations FuzzRetryAfter"
	"./web/app/http FuzzHostCheck"
	"./web/app/http FuzzSignInRedirect"
	"./web/app/servicetest FuzzBundleArchive"
)

failed=0
logs="$(mktemp -d)"
[ -n "$report" ] && : >"$report"

for entry in "${targets[@]}"; do
	pkg="${entry% *}"
	name="${entry#* }"
	start=$(date +%s)
	if go test -run '^$' -fuzz "^${name}\$" -fuzztime "$budget" -parallel "${FUZZ_PARALLEL:-4}" "$pkg" >"$logs/$name.log" 2>&1; then
		result=ok
	else
		result=FAIL
		failed=1
		tail -20 "$logs/$name.log"
	fi
	line="$name $pkg $(($(date +%s) - start))s $result"
	echo "$line"
	[ -n "$report" ] && echo "$line" >>"$report"
done

exit "$failed"
