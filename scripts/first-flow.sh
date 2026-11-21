#!/usr/bin/env bash
# The first complete flow of Stuarty (ADR-0031, PRD-004 section 3, epic 38), scripted: from an idea to a
# published short vertical video with captions and a thumbnail, through the CLI and the MCP server, with
# a fake AI provider and a fake social network on loopback. No network, no real account.
#
#   bash scripts/first-flow.sh
#
# It prints the address the fake network returned for the post, and exits 0 only when every step held:
# nothing left the machine before a person approved, the file that went out is the exported one, and the
# post is on the calendar as published.
#
# It owns everything it starts: its own workspace in a temporary folder, its own free port (never 7878,
# never 7979), a master key file next to the workspace, and it stops both servers when it ends.
# Environment (all optional):
#   STUARTY_BIN                a built stuarty binary; without it the script builds one
#   STUARTY_TEST_FFMPEG_DIR    the folder of the test FFmpeg (ffmpeg and ffprobe, or its bin/); without it
#                              ffmpeg and ffprobe are looked up in PATH
#   FIRST_FLOW_PORT            the port of the server (default: a random one between 40000 and 59999)
#   FIRST_FLOW_KEEP=1          keep the temporary folder and say where it is
#   TMPDIR                     where the temporary folder is made
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# The fake addresses (STUARTY_TEST_SOCIAL_URL) and the fake transcriber work only under APP_ENV=test, so
# the script runs under it and refuses another value rather than overriding what the caller chose.
if [ -n "${APP_ENV:-}" ] && [ "${APP_ENV}" != "test" ]; then
	echo "first-flow: APP_ENV is ${APP_ENV}; the fake network and the fake transcriber need APP_ENV=test" >&2
	exit 2
fi
export APP_ENV=test

native() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf '%s' "$1"; fi
}

die() {
	echo "first-flow: FAILED: $*" >&2
	exit 1
}

step() { echo "== $*" >&2; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/first-flow.XXXXXX")"
SERVER_PID=""
FAKE_PID=""

cleanup() {
	local code=$?
	if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" >/dev/null 2>&1 || true; fi
	if [ -n "$FAKE_PID" ]; then kill "$FAKE_PID" >/dev/null 2>&1 || true; fi
	# The servers hold their files open until they are gone.
	sleep 1
	if [ "${FIRST_FLOW_KEEP:-}" = 1 ]; then
		echo "first-flow: kept $WORK" >&2
	else
		rm -rf "$WORK" >/dev/null 2>&1 || true
	fi
	exit "$code"
}
trap cleanup EXIT

# ---- the tools: FFmpeg, the stuarty binary, the fake services --------------------------------------------

ffdir="${STUARTY_TEST_FFMPEG_DIR:-}"
exe=""
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) exe=".exe" ;; esac
if [ -n "$ffdir" ]; then
	if [ -x "$ffdir/bin/ffmpeg$exe" ]; then ffdir="$ffdir/bin"; fi
	FFMPEG="$ffdir/ffmpeg$exe"
	FFPROBE="$ffdir/ffprobe$exe"
else
	FFMPEG="$(command -v ffmpeg || true)"
	FFPROBE="$(command -v ffprobe || true)"
fi
[ -x "$FFMPEG" ] && [ -x "$FFPROBE" ] || die "no ffmpeg and ffprobe: set STUARTY_TEST_FFMPEG_DIR or put them in PATH"

mkdir -p "$WORK/bin"
STUARTY="${STUARTY_BIN:-}"
if [ -z "$STUARTY" ]; then
	step "building stuarty"
	(cd "$ROOT" && go build -o "$WORK/bin/stuarty$exe" ./web/cmd/app)
	STUARTY="$WORK/bin/stuarty$exe"
fi
(cd "$ROOT" && go build -o "$WORK/bin/firstflowfake$exe" ./internal/tools/firstflowfake)

# ---- the fake provider and the fake social network ---------------------------------------------------------

"$WORK/bin/firstflowfake$exe" -media-root "$(native "$WORK")" >"$WORK/fake.log" 2>&1 &
FAKE_PID=$!
for _ in $(seq 1 50); do
	grep -q '^ready ' "$WORK/fake.log" 2>/dev/null && break
	kill -0 "$FAKE_PID" 2>/dev/null || die "the fake services did not start: $(cat "$WORK/fake.log")"
	sleep 0.2
done
READY="$(grep '^ready ' "$WORK/fake.log")" || die "the fake services never said ready"
AI_URL="$(printf '%s' "$READY" | sed 's/.* provider=\([^ ]*\).*/\1/')"
SOCIAL_URL="$(printf '%s' "$READY" | sed 's/.* social=\([^ ]*\).*/\1/')"

# ---- the server ---------------------------------------------------------------------------------------------

HOME_DIR="$WORK/home"
mkdir -p "$HOME_DIR"
LAUNCHER_SECRET="first-flow-$RANDOM-$RANDOM-$RANDOM"
export STUARTY_HOME
STUARTY_HOME="$(native "$HOME_DIR")"
export STUARTY_MASTER_KEY_FILE
STUARTY_MASTER_KEY_FILE="$(native "$WORK/master.key")"
export STUARTY_FFMPEG STUARTY_FFPROBE
STUARTY_FFMPEG="$(native "$FFMPEG")"
STUARTY_FFPROBE="$(native "$FFPROBE")"
export STUARTY_TEST_SOCIAL_URL="$SOCIAL_URL"
export STUARTY_TEST_TRANSCRIBER=stub
export STUARTY_LAUNCHER_SECRET="$LAUNCHER_SECRET"
unset VUKEM_VIDEO_HOME STUARTY_PROJECT

PORT="${FIRST_FLOW_PORT:-$((40000 + RANDOM % 20000))}"
case "$PORT" in 7878 | 7979) die "port $PORT belongs to a person's own Stuarty" ;; esac
export STUARTY_ADDR="127.0.0.1:$PORT"
BASE="http://127.0.0.1:$PORT"

step "starting the server on $STUARTY_ADDR with the workspace $STUARTY_HOME"
"$STUARTY" serve >"$WORK/serve.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 150); do
	curl -sf "$BASE/up" >/dev/null 2>&1 && break
	kill -0 "$SERVER_PID" 2>/dev/null || die "the server stopped: $(tail -n 5 "$WORK/serve.log")"
	sleep 0.2
done
curl -sf "$BASE/up" >/dev/null 2>&1 || die "the server did not answer: $(tail -n 5 "$WORK/serve.log")"

# ---- helpers ------------------------------------------------------------------------------------------------

stuarty() { "$STUARTY" "$@"; }

# field NAME: the first string value of NAME in the JSON on standard input.
field() { grep -o "\"$1\":\"[^\"]*\"" | head -n 1 | cut -d'"' -f4; }
# number NAME: the first number value of NAME.
number() { grep -o "\"$1\":[0-9]*" | head -n 1 | cut -d: -f2; }

need() {
	[ -n "$2" ] || die "$1 not found in: $3"
	printf '%s' "$2"
}

# utc SECONDS: the UTC instant SECONDS after the epoch, as RFC 3339; GNU date (Linux, Git Bash) or BSD date (macOS).
utc() {
	date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r "$1" +%Y-%m-%dT%H:%M:%SZ
}

# mcp TOOL ARGS: one call of an MCP tool of `stuarty mcp`, the answer line of the call.
mcp() {
	local out
	out="$(printf '%s\n' \
		'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"first-flow","version":"1"}}}' \
		'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
		"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" |
		stuarty mcp | grep '"id":2')" || die "no answer from the MCP tool $1"
	case "$out" in *'"isError":true'* | *'"error":{'*) die "the MCP tool $1 failed: $out" ;; esac
	printf '%s' "$out"
}

# A person's session, the way the desktop app signs one in: the launcher secret the server was told, then the
# cookie of the launch code. The local token an agent holds cannot decide in the approval queue.
TOKEN="$(tr -d '\r\n' <"$HOME_DIR/token")"
launch="$(curl -sf -X POST "$BASE/api/v1/system/launch" -H "Authorization: Bearer $TOKEN" \
	-H "Stuarty-Launcher: $LAUNCHER_SECRET")" || die "the launch of a person session was refused"
curl -sf -c "$WORK/jar.txt" -o /dev/null "$(printf '%s' "$launch" | field url)" || die "no person session"

person() {
	local method="$1" path="$2" body="${3:-}"
	if [ -n "$body" ]; then
		curl -sf -b "$WORK/jar.txt" -X "$method" "$BASE/api/v1$path" -H "Origin: $BASE" \
			-H "Content-Type: application/json" -d "$body" || die "$method $path as a person failed"
	else
		curl -sf -b "$WORK/jar.txt" -X "$method" "$BASE/api/v1$path" -H "Origin: $BASE" \
			-H "Content-Type: application/json" || die "$method $path as a person failed"
	fi
}

uploads() { curl -sf "$SOCIAL_URL/_uploads"; }
upload_count() { uploads | grep -o '"id":"ff' | wc -l | tr -d ' '; }

revision() { stuarty project:show "$P" --json | number revision; }

# ---- 1. a project, and the briefing ------------------------------------------------------------------------

step "1. create the project and answer the briefing (MCP)"
# A project has a description and a SMART goal, and no timeline until a video is added (SDD-005 section 5.12.2).
created="$(mcp project_create '{"name":"Sourdough short","description":"A ten second vertical short on baking a first sourdough loaf, for home bakers, with captions and a cover.","goal":{"specific":"Publish one short on the first sourdough loaf","measurable":"A thousand views and twenty saves in a week","achievable":"The footage and the recipe already exist","relevant":"Home bakers ask how to start with sourdough","time_bound":"Script today, edit tomorrow, publish this week","due":"2999-12-31"}}')"
P="$(need project_id "$(printf '%s' "$created" | field project_id)" "$created")"
mcp project_item "{\"action\":\"add\",\"project_id\":\"$P\",\"kind\":\"timeline\",\"title\":\"Sourdough short\",\"preset\":\"9:16\"}" >/dev/null
mcp project_item "{\"action\":\"briefing\",\"project_id\":\"$P\",\"answers\":{\"audience\":\"Home bakers who want a first sourdough loaf\",\"goal\":\"Watch to the end and try the recipe\",\"platform\":\"youtube-shorts\",\"format\":\"vertical-9x16\",\"duration\":\"10\",\"tone\":\"warm\"}}" >/dev/null
stuarty brief:show --project "$P" --json | grep -q 'first sourdough loaf' || die "the brief does not hold the answers"

# ---- 2. the script: drafted by a model, then edited ---------------------------------------------------------

step "2. connect the fake AI provider, draft the script (a person approves), edit it"
conn="$(printf 'sk-first-flow-fake-key' | stuarty connection:add --kind openai --label "Fake AI" \
	--endpoint "$AI_URL" --accepts text --json)"
AI="$(need connection_id "$(printf '%s' "$conn" | field id)" "$conn")"
person PUT /providers/prices '{"provider":"openai","model":"fake-small","currency":"USD","input_micros_per_mtok":1000000,"output_micros_per_mtok":2000000}' >/dev/null

printf '# Hook\nVO: placeholder\n' >"$WORK/script0.md"
item="$(stuarty item:add --project "$P" --kind text --title Script --text-kind script --text "$(native "$WORK/script0.md")" --json)"
SCRIPT="$(need item_id "$(printf '%s' "$item" | grep -o '"id":"pit_[a-z0-9]*"' | head -n 1 | cut -d'"' -f4)" "$item")"

draft="$(mcp job_start "{\"project_id\":\"$P\",\"kind\":\"text.draft\",\"args\":{\"item_id\":\"$SCRIPT\",\"connection_id\":\"$AI\",\"model\":\"fake-small\",\"instruction\":\"A 10 second script about baking a first sourdough loaf\"}}")"
DRAFT_JOB="$(need job_id "$(printf '%s' "$draft" | field job_id)" "$draft")"
DRAFT_APPROVAL="$(need approval_id "$(printf '%s' "$draft" | field approval_id)" "$draft")"
# The agent cannot approve what it asked for; a person does.
if stuarty approval:decide "$DRAFT_APPROVAL" --approve >/dev/null 2>&1; then
	die "the local token approved a request: only a person may"
fi
person POST "/approvals/$DRAFT_APPROVAL/decide" '{"decision":"approve"}' >/dev/null
done_job="$(stuarty job:show "$DRAFT_JOB" --wait --timeout 60s --json)"
case "$done_job" in *'"state":"succeeded"'*) ;; *) die "the draft job did not succeed: $done_job" ;; esac
stuarty text:show "$SCRIPT" --project "$P" --json | grep -q 'Sourdough, from starter to slice' || die "the draft is not in the script"
printf 'Sourdough, from starter to slice.\n\nFeed the starter, fold the dough, bake it hot.\n' >"$WORK/script1.md"
edited="$(stuarty text:write "$SCRIPT" --project "$P" --text "$(native "$WORK/script1.md")" --base 2 --json)"
[ "$(printf '%s' "$edited" | number revision)" = 3 ] || die "the edited script is not revision 3: $edited"

# ---- 3. the footage and the cut -----------------------------------------------------------------------------

step "3. import the footage and cut it to a vertical short"
"$FFMPEG" -v error -y -f lavfi -i "testsrc2=size=360x640:rate=30:duration=6" -f lavfi -i "sine=frequency=440:duration=6" \
	-c:v mpeg4 -q:v 5 -c:a aac -shortest "$WORK/footage.mp4"
imported="$(stuarty media:import "$(native "$WORK/footage.mp4")" --project "$P" --wait --json)"
ASSET="$(need asset_id "$(printf '%s' "$imported" | field asset_id)" "$imported")"
case "$imported" in *'"state":"ready"'*) ;; *) die "the footage is not ready: $imported" ;; esac

cut="$(mcp edit "{\"project_id\":\"$P\",\"base_revision\":$(revision),\"operations\":[{\"op\":\"track.add\",\"args\":{\"kind\":\"visual\",\"name\":\"Video\",\"id\":\"trk_v2dec00001\"}},{\"op\":\"clip.add\",\"args\":{\"track_id\":\"trk_v2dec00001\",\"asset_id\":\"$ASSET\",\"start\":\"0s\",\"source_in\":\"0s\",\"duration\":\"4s\",\"fit\":\"cover\",\"id\":\"itm_c2mp000001\"}}]}")"
case "$cut" in *'"applied":true'*) ;; *) die "the cut was not applied: $cut" ;; esac

# ---- 4. captions from the transcript ------------------------------------------------------------------------

step "4. captions from the transcript"
stuarty transcribe --project "$P" --asset "$ASSET" --language en --wait --json | grep -q '"state":"succeeded"' || die "the transcription failed"
caps="$(mcp edit "{\"project_id\":\"$P\",\"base_revision\":$(revision),\"operations\":[{\"op\":\"track.add\",\"args\":{\"kind\":\"visual\",\"name\":\"Captions\",\"id\":\"trk_cap7ts0001\"}},{\"op\":\"caption.from_transcript\",\"args\":{\"clip_item_id\":\"itm_c2mp000001\",\"track_id\":\"trk_cap7ts0001\",\"new_ids\":[\"itm_cap0000001\",\"itm_cap0000002\"]}}]}")"
case "$caps" in *'"applied":true'*'caption'*) ;; *) die "the captions were not applied: $caps" ;; esac
lint="$(stuarty lint --project "$P" --json)"
[ "$(printf '%s' "$lint" | number errors)" = 0 ] || die "the project has lint errors: $lint"

# ---- 5. the thumbnail, from a template ---------------------------------------------------------------------

step "5. a thumbnail from a template"
tpl="$(stuarty image:create --preset vertical_cover --name "Cover template" --template --json)"
TPL="$(need image_id "$(printf '%s' "$tpl" | field image_id)" "$tpl")"
stuarty image:edit "$TPL" layer.add_fill color='#1B1B2F' --json >/dev/null
stuarty image:edit "$TPL" layer.add_text text=Title slot=title style:='{"size":160,"color":"#FFE600","font_weight":700}' --json >/dev/null
cover="$(printf '{"title":{"text":"First sourdough"}}' | stuarty image:create --from-template "$TPL" --name "Sourdough cover" --values - --json)"
COVER="$(need image_id "$(printf '%s' "$cover" | field image_id)" "$cover")"
cover_png="$(stuarty image:export "$COVER" --format png --json)"
[ "$(printf '%s' "$cover_png" | number bytes)" -gt 1000 ] || die "the cover png is empty: $cover_png"
# The export of the image is a job, and the publication takes that job as its thumbnail (D-111).
COVER_JOB="$(need job_id "$(printf '%s' "$cover_png" | field job_id)" "$cover_png")"

# ---- 6. the export with a platform preset --------------------------------------------------------------------

step "6. export with the YouTube Shorts preset"
exp="$(mcp job_start "{\"project_id\":\"$P\",\"kind\":\"export\",\"args\":{\"preset\":\"youtube-shorts\"},\"wait_ms\":25000}")"
EXPORT_JOB="$(need job_id "$(printf '%s' "$exp" | field job_id)" "$exp")"
for _ in $(seq 1 60); do
	got="$(mcp job_get "{\"job_id\":\"$EXPORT_JOB\",\"wait_ms\":5000}")"
	case "$got" in *'"state":"succeeded"'*) break ;; *'"state":"failed"'*) die "the export failed: $got" ;; esac
done
case "$got" in *'"state":"succeeded"'*) ;; *) die "the export did not finish: $got" ;; esac
case "$got" in *'"ok":false'*) die "a check of the export failed: $got" ;; esac
case "$got" in *'fits YouTube Shorts'*) ;; *) die "the export was not checked against the preset: $got" ;; esac
VIDEO_BYTES="$(printf '%s' "$got" | number bytes)"
[ "${VIDEO_BYTES:-0}" -gt 10000 ] || die "the exported video is too small: $got"

# ---- 7. the account ------------------------------------------------------------------------------------------

step "7. connect the fake network account with a developer app of the person"
start="$(printf 'first-flow-client-secret' | stuarty social:connect --network fake --client-id first-flow-client \
	--redirect-uri "$BASE/social/callback" --json)"
SOCIAL_CONN="$(need connection_id "$(printf '%s' "$start" | field connection_id)" "$start")"
authorize="$(printf '%s' "$start" | field authorize_url)"
# The consent screen answers with a redirect that carries the code and the state back to the app.
location="$(curl -s -i "$authorize" | grep -i '^location:' | tr -d '\r' | head -n 1)"
code="$(printf '%s' "$location" | sed 's/.*[?&]code=\([^&]*\).*/\1/')"
state="$(printf '%s' "$location" | sed 's/.*[?&]state=\([^&]*\).*/\1/')"
[ -n "$code" ] && [ -n "$state" ] || die "the consent screen did not redirect with a code: $location"
person POST /social/complete "{\"state\":\"$state\",\"code\":\"$code\"}" >/dev/null
stuarty social:account:list --json | grep -q '"state":"ready"' || die "the account is not signed in"

# ---- 8. the publication: create, schedule, approve ---------------------------------------------------------

step "8. create the publication, schedule it, and approve it as a person"
# The time is a few seconds ahead, so the post is due while its request still waits: it must not go out, even
# past its time, until a person approves (ADR-0032). The approval comes inside the grace of a minute
# (core/publish), so the post then goes out at once.
AT="$(utc $(($(date -u +%s) + 5)))"
pub="$(stuarty publication:create --project "$P" --title "Sourdough short" \
	--text "From starter to slice in four seconds #sourdough" --export "$EXPORT_JOB" --thumbnail "$COVER_JOB" \
	--connection "$SOCIAL_CONN" --at "$AT" --made-for-kids no --visibility public --json)"
PUB="$(need publication "$(printf '%s' "$pub" | field id)" "$pub")"
[ "$(printf '%s' "$pub" | field thumbnail)" = "$COVER_JOB" ] || die "the cover is not the thumbnail of the post: $pub"
[ "$(upload_count)" = 0 ] || die "something was sent before the post was scheduled"
sched="$(stuarty publication:schedule "$PUB" --json)"
SCHEDULED="$(date -u +%s)"
case "$sched" in *'"state":"waiting_approval"'*) ;; *) die "the scheduled post is not waiting for approval: $sched" ;; esac
PUB_APPROVAL="$(need approval_id "$(printf '%s' "$sched" | field approval_id)" "$sched")"
if stuarty approval:decide "$PUB_APPROVAL" --approve >/dev/null 2>&1; then
	die "the local token approved a publication: only a person may"
fi
# Nothing goes out while the request waits. The scheduler of the server looks at the calendar when it changes
# and at least every 30 seconds (service/publish), so watching for 32 seconds after the schedule spans one of
# its passes made after the time of the post: a scheduler that skipped the approval would have sent it.
while [ "$(date -u +%s)" -lt $((SCHEDULED + 32)) ]; do
	[ "$(upload_count)" = 0 ] || die "a post went out before a person approved it"
	sleep 2
done
[ "$(upload_count)" = 0 ] || die "a post went out before a person approved it"
waiting="$(stuarty publication:show "$PUB" --json)"
case "$waiting" in
*'"state":"waiting_approval"'*) ;;
*) die "past its time, the post no longer waits for a person: $waiting" ;;
esac
person POST "/approvals/$PUB_APPROVAL/decide" '{"decision":"approve"}' >/dev/null

# ---- 9. published, once, after the approval ----------------------------------------------------------------

step "9. wait for the post to go out"
shown=""
for _ in $(seq 1 60); do
	shown="$(stuarty publication:show "$PUB" --json)"
	case "$shown" in *'"state":"published"'*) break ;; *'"state":"failed"'*) die "the post failed: $shown" ;; esac
	sleep 2
done
case "$shown" in *'"state":"published"'*) ;; *) die "the post was not published in time: $shown" ;; esac
case "$shown" in *"\"approval_id\":\"$PUB_APPROVAL\""*) ;; *) die "the published post does not carry the approval of the person: $shown" ;; esac
POST_URL="$(need url "$(printf '%s' "$shown" | field url)" "$shown")"
[ "$(upload_count)" = 1 ] || die "the network received $(upload_count) uploads, want 1"
[ "$(uploads | number bytes)" = "$VIDEO_BYTES" ] || die "the file that went out is not the exported video"

# ---- 10. the calendar -----------------------------------------------------------------------------------------

step "10. the post is on the calendar"
now="$(date -u +%s)"
from="$(utc $((now - 86400)))"
until_="$(utc $((now + 86400)))"
cal="$(stuarty publication:list --from "$from" --until "$until_" --json)"
case "$cal" in *"$PUB"*'"state":"published"'*) ;; *) die "the post is not on the calendar as published: $cal" ;; esac

echo "first-flow: OK" >&2
echo "$POST_URL"
