#!/usr/bin/env bash
# The recipe of a long talking video (epic 39, SDD-018 section 17), scripted: a five-minute talk with a pause in
# every ten seconds, an opening made from the built-in template Long video opening, the pauses cut out, captions
# after the cut, chapters as markers and as a list, a cover from the template Long video thumbnail, the export,
# and a short of thirty seconds taken from the edit in a copy of the project, refit to 9:16 and exported. It
# follows the steps of the skill long-talking-video with the operations the skill names. No network, no account;
# the transcript is the fixed stub of the test transcriber (four words in the first two seconds).
#
#   bash scripts/recipe-long-video.sh
#
# It prints the two files and exits 0 only when each one was exported, passed the checks of its export and has the
# shape and the length the recipe promises. It owns everything it starts: its own workspace in a temporary folder
# and its own free port (never 7878, never 7979), and it stops the server when it ends.
# Environment: see scripts/lib/recipe-common.sh.
set -euo pipefail

RECIPE=recipe-long-video
# shellcheck source=scripts/lib/recipe-common.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/recipe-common.sh"

recipe_start

# created_ids JSON: the ids the answer of an edit lists under "created", one for each line.
created_ids() { grep -o '"created":\[[^]]*\]' | head -n 1 | grep -o 'itm_[0-9a-z]*\|trk_[0-9a-z]*'; }

# items PROJECT: one line for each item of the timeline: id, type, first frame and length in frames.
items() {
	local out
	out="$(stuarty project:show "$1" --detail full --limit 500 --json)"
	case "$out" in *'"next_cursor":null'*) ;; *) die "the item list of $1 has more pages: raise the limit" ;; esac
	printf '%s' "$out" | grep -o 'itm_[0-9a-z]* [a-z]* [0-9.]*s/[0-9]*f +[0-9.]*s/[0-9]*f' |
		sed 's|^\(itm_[0-9a-z]*\) \([a-z]*\) [0-9.]*s/\([0-9]*\)f +[0-9.]*s/\([0-9]*\)f$|\1 \2 \3 \4|'
}

# ids_json ID...: a JSON list of the ids.
ids_json() {
	local out="" id
	for id in "$@"; do out="$out${out:+,}\"$id\""; done
	printf '[%s]' "$out"
}

# frames PROJECT: the length of the timeline in frames.
frames() { stuarty project:show "$1" --json | number duration_frames; }

# ---- 1. the brief --------------------------------------------------------------------------------------------

step "1. a project, and the brief of the long video"
# A project has a description and a SMART goal, and no timeline until one is added (SDD-005 section 5.12.2).
created="$(stuarty project:create --name "A year of videos" --json \
	--description "A long horizontal talking video on how a creator plans a year of weekly videos, with chapters." \
	--specific "Publish one long video about planning a year" --measurable "Five hundred views and fifty comments" \
	--achievable "The talk is recorded and the outline exists" --relevant "Viewers ask how the weekly plan is made" \
	--time-bound "Outline this week, edit next week, then publish" --due 2999-12-31)"
P="$(need project_id "$(printf '%s' "$created" | field project_id)" "$created")"
stuarty item:add --project "$P" --kind timeline --title "A year of videos" --preset 16:9 >/dev/null
cat >"$WORK/brief.md" <<'EOF'
# Brief

## Topic

How a creator plans a year of weekly videos.

## Audience

Creators who publish every week and run out of ideas.

## Goal

The viewer leaves with a plan they can start this week.

## Speakers

Ana Souza, alone, on camera.

## Platform

youtube

## Format

horizontal-16x9

## Duration (seconds)

220

## Cut style

natural

## Short from it

yes
EOF
stuarty brief:set --project "$P" --text "$(native "$WORK/brief.md")" --json >/dev/null
stuarty brief:show --project "$P" --json | grep -q 'plans a year' || die "the brief does not hold the answers"

# ---- 2. the footage ------------------------------------------------------------------------------------------

step "2. a five-minute talk with three seconds of silence in every ten, imported and transcribed"
"$FFMPEG" -v error -y -f lavfi -i "testsrc2=size=640x360:rate=15:duration=300" -f lavfi -i "sine=frequency=300:duration=300" \
	-af "volume=enable='gte(mod(t,10),7)':volume=0" -c:v mpeg4 -b:v 150k -c:a aac -b:a 48k -shortest "$WORK/talk.mp4"
imported="$(stuarty media:import "$(native "$WORK/talk.mp4")" --project "$P" --wait --json)"
case "$imported" in *'"state":"ready"'*) ;; *) die "the talk is not ready: $imported" ;; esac
ASSET="$(need asset_id "$(printf '%s' "$imported" | field asset_id)" "$imported")"
stuarty transcribe --project "$P" --asset "$ASSET" --language en --wait --json | grep -q '"state":"succeeded"' || die "the transcription failed"
stuarty media:inspect "$ASSET" --project "$P" --sections transcript,silences | grep -q 'hello brave new world' || die "the transcript is not readable"

# ---- 3. the opening, from a template -------------------------------------------------------------------------

step "3. the opening from the template Long video opening"
templates="$(stuarty template:list --json)"
TPL="$(printf '%s' "$templates" | grep -o '"template_id":"tpl_[a-z0-9]*","name":"Long video opening"' | field template_id)"
need "the template Long video opening" "$TPL" "$templates" >/dev/null
cat >"$WORK/fill.json" <<EOF
{
  "cold_open": {"asset_id": "$ASSET", "source_in": "20s"},
  "title": {"text": "How I plan a year of videos"},
  "subtitle": {"text": "For creators who publish every week"},
  "speaker": {"text": "Ana Souza, video creator"}
}
EOF
made="$(stuarty template:new "$TPL" --name "A year of videos, edit" --fill "$(native "$WORK/fill.json")" --json)"
L="$(need project_id "$(printf '%s' "$made" | field project_id)" "$made")"
case "$made" in *'"unfilled":[]'*) ;; *) die "a slot of the template is not filled: $made" ;; esac
stuarty brief:set --project "$L" --text "$(native "$WORK/brief.md")" --json >/dev/null

# ---- 4. the main cut -----------------------------------------------------------------------------------------

step "4. the talk after the opening, and the dead air out"
full="$(stuarty project:show "$L" --detail full --json)"
FOOTAGE="$(need "the Footage track" "$(printf '%s' "$full" | grep -o 'trk_[0-9a-z]* [a-z]* visual \\"Footage\\"' | head -n 1 | cut -d' ' -f1)" "$full")"
added="$(stuarty edit --project "$L" clip.add track_id="$FOOTAGE" asset_id="$ASSET" start=10s source_in=0s duration=290s --json)"
CLIP="$(need "the id of the clip" "$(printf '%s' "$added" | created_ids | head -n 1)" "$added")"
before="$(frames "$L")"
cut="$(stuarty edit --project "$L" clip.remove_silences item_id="$CLIP" --json)"
case "$cut" in *'"applied":true'*) ;; *) die "the silences were not removed: $cut" ;; esac
pieces="$(printf '%s' "$cut" | created_ids | wc -l | tr -d ' ')"
[ "$pieces" -ge 25 ] || die "only $pieces pieces were made from the talk, want at least 25 (one for each pause)"
after="$(frames "$L")"
# 29 pauses of about three seconds are gone: the edit is about 80 seconds shorter than the 300 seconds it started with.
[ "$after" -lt $((before - 2000)) ] || die "the cut left $after frames of $before: the pauses are still there"

# ---- 5. captions, after the cut ------------------------------------------------------------------------------

step "5. captions from the transcript"
track="$(stuarty edit --project "$L" track.add kind=visual name=Captions --json)"
CAPTIONS="$(need "the captions track" "$(printf '%s' "$track" | created_ids | head -n 1)" "$track")"
caps="$(stuarty edit --project "$L" caption.from_transcript clip_item_id="$CLIP" track_id="$CAPTIONS" --json)"
case "$caps" in *'"applied":true'*'"words_used":4'*) ;; *) die "the captions were not made from the transcript: $caps" ;; esac

# ---- 6. chapters ---------------------------------------------------------------------------------------------

step "6. chapters as markers, and the list for the description"
cat >"$WORK/chapters.txt" <<'EOF'
00:00 The plan in one minute
01:10 Why a year and not a month
02:00 Three themes for every quarter
02:50 What to do when you miss a week
EOF
n=0
while IFS= read -r line; do
	n=$((n + 1))
	stamp="${line%% *}"
	title="${line#* }"
	seconds=$((10#${stamp%%:*} * 60 + 10#${stamp##*:}))
	# The first chapter is at the start of the video; the others are at their times on the edited timeline.
	marked="$(stuarty edit --project "$L" marker.add at="${seconds}s" label="$title" --json)"
	case "$marked" in *'"applied":true'*) ;; *) die "the marker of chapter $n was not added: $marked" ;; esac
done <"$WORK/chapters.txt"
[ "$n" = 4 ] || die "$n chapters, want 4"
stuarty item:add --project "$L" --kind text --title "Chapters" --text "$(native "$WORK/chapters.txt")" --json >/dev/null
stuarty text:list --project "$L" --json | grep -q '"title":"Chapters"' || die "the chapter list is not a text of the project"
head -n 1 "$WORK/chapters.txt" | grep -q '^00:00 ' || die "the chapter list does not start at 00:00"

# ---- 7. check, cover, export ---------------------------------------------------------------------------------

step "7. lint, the cover from the template Long video thumbnail, and the export"
lint="$(stuarty lint --project "$L" --json)"
[ "$(printf '%s' "$lint" | number errors)" = 0 ] || die "the edit has lint errors: $lint"
LONG_FILE="$(export_file "$L")"
TOTAL="$(frames "$L")"

# The cover: a still of the edit, brought in as a picture for the template.
stuarty export --project "$L" --format png --at 20s --out "$(native "$WORK/still.png")" --wait --json >/dev/null
still="$(stuarty media:import "$(native "$WORK/still.png")" --project "$P" --wait --json)"
HASH="$(need "the hash of the still" "$(printf '%s' "$still" | field hash)" "$still")"
images="$(stuarty image:list --templates only --json)"
IMG_TPL="$(printf '%s' "$images" | grep -o '"image_id":"img_[a-z0-9]*","name":"Long video thumbnail"' | field image_id)"
need "the template Long video thumbnail" "$IMG_TPL" "$images" >/dev/null
cover="$(printf '{"picture":{"asset":"%s"},"title":{"text":"A year of videos"},"subtitle":{"text":"A plan for every week"}}' "$HASH" |
	stuarty image:create --from-template "$IMG_TPL" --name "A year of videos, cover" --values - --json)"
COVER="$(need image_id "$(printf '%s' "$cover" | field image_id)" "$cover")"
cover_png="$(stuarty image:export "$COVER" --format png --json)"
[ "$(printf '%s' "$cover_png" | number bytes)" -gt 1000 ] || die "the cover png is empty: $cover_png"

# ---- 8. a short from the long video --------------------------------------------------------------------------

step "8. a short of thirty seconds from the edit, in a copy of the project"
copy="$(stuarty project:duplicate "$L" --name "A year of videos, short" --json)"
S="$(need project_id "$(printf '%s' "$copy" | field project_id)" "$copy")"
START=1200 # frame 40 s
END=2100   # frame 70 s

# split_at FRAME: cuts the one clip that spans the frame in the copy.
split_at() {
	local id kind first length
	while read -r id kind first length; do
		if [ "$first" -lt "$1" ] && [ $((first + length)) -gt "$1" ]; then
			applied="$(stuarty edit --project "$S" item.split item_id="$id" at="${1}f" --json)"
			case "$applied" in *'"applied":true'*) return 0 ;; *) die "the cut at frame $1 was not applied: $applied" ;; esac
		fi
	done < <(items "$S")
	die "no item spans the frame $1"
}

# delete_where RIPPLE TEST: deletes the items for which TEST (a shell test on $first and $length) holds.
delete_where() {
	local ripple="$1" test="$2" id kind first length ids=()
	while read -r id kind first length; do
		if eval "$test"; then ids+=("$id"); fi
	done < <(items "$S")
	[ "${#ids[@]}" -gt 0 ] || die "nothing to delete ($test)"
	printf '{"operations":[{"op":"item.delete","args":{"item_ids":%s,"ripple":"%s"}}]}' "$(ids_json "${ids[@]}")" "$ripple" >"$WORK/delete.json"
	applied="$(stuarty edit --project "$S" --ops "$(native "$WORK/delete.json")" --json)"
	case "$applied" in *'"applied":true'*) ;; *) die "the delete ($test) was not applied: $applied" ;; esac
}

split_at "$END"
delete_where none '[ "$first" -ge "$END" ]'
split_at "$START"
delete_where all '[ $((first + length)) -le "$START" ]'
[ "$(frames "$S")" = $((END - START)) ] || die "the short is $(frames "$S") frames, want $((END - START))"
reframed="$(stuarty edit --project "$S" settings.reframe aspect=9:16 --json)"
case "$reframed" in *'"applied":true'*) ;; *) die "the reframe to 9:16 was not applied: $reframed" ;; esac
[ "$(printf '%s' "$(stuarty lint --project "$S" --json)" | number errors)" = 0 ] || die "the short has lint errors"
SHORT_FILE="$(export_file "$S")"

# ---- 9. the two files ---------------------------------------------------------------------------------------

step "9. check the two files"
check_file() {
	local label="$1" file="$2" want="$3" frames_want="$4" got seconds expect
	[ -s "$file" ] || die "no file for $label: $file"
	got="$(probe "$file")"
	case "$got" in "$want "*) ;; *) die "the $label file is $got, want $want" ;; esac
	seconds="${got#* }"
	expect="$(awk -v f="$frames_want" 'BEGIN { printf "%.1f", f / 30 }')"
	awk -v s="$seconds" -v e="$expect" 'BEGIN { d = s - e; if (d < 0) d = -d; exit !(d < 0.2) }' ||
		die "the $label file is $seconds seconds, want $expect"
	echo "$label  $want  ${seconds}s  $file"
}
check_file "long video" "$LONG_FILE" 1920x1080 "$TOTAL"
check_file "short" "$SHORT_FILE" 1080x1920 $((END - START))

echo "recipe-long-video: OK" >&2
