#!/usr/bin/env bash
# The recipe of a product commercial (epic 39, SDD-018 section 17), scripted: one brief, a timeline made from the
# built-in template Commercial 15 seconds, and one exported file per aspect ratio (9:16, 1:1, 16:9). It follows the
# steps of the skill product-commercial with the operations the skill names. No network, no account.
#
#   bash scripts/recipe-commercial.sh
#
# It prints the three files and exits 0 only when each one was exported, passed the checks of its export and has the
# shape and the length the recipe promises. It owns everything it starts: its own workspace in a temporary folder
# and its own free port (never 7878, never 7979), and it stops the server when it ends.
# Environment: see scripts/lib/recipe-common.sh.
set -euo pipefail

RECIPE=recipe-commercial
# shellcheck source=scripts/lib/recipe-common.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/recipe-common.sh"

recipe_start

# ---- 1. the brief ----------------------------------------------------------------------------------------------

step "1. a project, and the brief of the commercial"
created="$(stuarty project:create --name "Keep mug commercial" --preset 9:16 --json)"
P="$(need project_id "$(printf '%s' "$created" | field project_id)" "$created")"
cat >"$WORK/brief.md" <<'EOF'
# Brief

## Product

Keep, a steel mug that holds heat for eight hours.

## Audience

Commuters who drink coffee on the way to work.

## Problem

Their coffee goes cold before they finish it.

## Offer

20% off this week.

## Call to action

Shop now.

## Platform

youtube-shorts

## Format

vertical-9x16

## Duration (seconds)

15
EOF
stuarty brief:set --project "$P" --text "$(native "$WORK/brief.md")" --json >/dev/null
stuarty brief:show --project "$P" --json | grep -q 'steel mug' || die "the brief does not hold the answers"

# ---- 3. the shots ----------------------------------------------------------------------------------------------

step "2. import three shots of the product"
declare -a SHOT
n=0
for source in "testsrc2=size=360x640:rate=30:duration=6" "rgbtestsrc=size=360x640:rate=30:duration=6" "smptebars=size=360x640:rate=30:duration=6"; do
	n=$((n + 1))
	"$FFMPEG" -v error -y -f lavfi -i "$source" -f lavfi -i "sine=frequency=$((300 + n * 100)):duration=6" \
		-c:v mpeg4 -q:v 5 -c:a aac -shortest "$WORK/shot$n.mp4"
	imported="$(stuarty media:import "$(native "$WORK/shot$n.mp4")" --project "$P" --wait --json)"
	case "$imported" in *'"state":"ready"'*) ;; *) die "shot $n is not ready: $imported" ;; esac
	SHOT[n]="$(need asset_id "$(printf '%s' "$imported" | field asset_id)" "$imported")"
done

# ---- 4. the timeline, from a template ------------------------------------------------------------------------------

step "3. the timeline from the template Commercial 15 seconds"
templates="$(stuarty template:list --json)"
TPL="$(printf '%s' "$templates" | grep -o '"template_id":"tpl_[a-z0-9]*","name":"Commercial 15 seconds"' | field template_id)"
need "the template Commercial 15 seconds" "$TPL" "$templates" >/dev/null
cat >"$WORK/fill.json" <<EOF
{
  "hook_shot": {"asset_id": "${SHOT[1]}"},
  "problem_shot": {"asset_id": "${SHOT[2]}"},
  "product_shot": {"asset_id": "${SHOT[3]}"},
  "hook": {"text": "Cold coffee again?"},
  "problem": {"text": "Your coffee goes cold in minutes"},
  "product_line": {"text": "Keep holds heat for 8 hours"},
  "offer": {"text": "20% off this week"},
  "call_to_action": {"text": "Shop now"}
}
EOF
made="$(stuarty template:new "$TPL" --name "Keep mug 9x16" --fill "$(native "$WORK/fill.json")" --json)"
MASTER="$(need project_id "$(printf '%s' "$made" | field project_id)" "$made")"
case "$made" in *'"unfilled":[]'*) ;; *) die "a slot of the template is not filled: $made" ;; esac
# The brief of the first project goes with the production: the project of the template carries on from here.
stuarty brief:set --project "$MASTER" --text "$(native "$WORK/brief.md")" --json >/dev/null

# ---- 5. check -------------------------------------------------------------------------------------------------------

step "4. lint: no error before the export"
lint="$(stuarty lint --project "$MASTER" --json)"
[ "$(printf '%s' "$lint" | number errors)" = 0 ] || die "the master has lint errors: $lint"

# ---- 6. one version for each aspect ratio ---------------------------------------------------------------------------

step "5. export the master (9:16), then a copy reframed to 1:1 and one to 16:9"
declare -A FILE
FILE[9:16]="$(export_file "$MASTER")"
for aspect in 1:1 16:9; do
	copy="$(stuarty project:duplicate "$MASTER" --name "Keep mug $aspect" --json)"
	COPY="$(need project_id "$(printf '%s' "$copy" | field project_id)" "$copy")"
	reframed="$(stuarty edit --project "$COPY" settings.reframe aspect="$aspect" --json)"
	case "$reframed" in *'"applied":true'*) ;; *) die "the reframe to $aspect was not applied: $reframed" ;; esac
	[ "$(printf '%s' "$(stuarty lint --project "$COPY" --json)" | number errors)" = 0 ] || die "the $aspect copy has lint errors"
	FILE[$aspect]="$(export_file "$COPY")"
done

# ---- 7. the three files ---------------------------------------------------------------------------------------------

step "6. check the three files"
check_file() {
	local aspect="$1" want="$2" got seconds
	[ -s "${FILE[$aspect]}" ] || die "no file for $aspect: ${FILE[$aspect]}"
	got="$(probe "${FILE[$aspect]}")"
	case "$got" in "$want "*) ;; *) die "the $aspect file is $got, want $want" ;; esac
	seconds="${got#* }"
	# The template is 15 seconds long.
	case "$seconds" in 14.9* | 15.0*) ;; *) die "the $aspect file is $seconds seconds, want 15" ;; esac
	echo "$aspect  $want  ${seconds}s  ${FILE[$aspect]}"
}
check_file 9:16 1080x1920
check_file 1:1 1080x1080
check_file 16:9 1920x1080

echo "recipe-commercial: OK" >&2
