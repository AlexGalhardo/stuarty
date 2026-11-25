#!/usr/bin/env bash
# Builds a workspace with a released binary, running the fixed scenario of P4-SRV-001.
#
#   build-workspace.sh <binary> <workspace dir> <media file>
#
# The media file must be a real video; make one with:
#   ffmpeg -f lavfi -i testsrc=size=320x240:rate=30:duration=2 -f lavfi -i sine=duration=2 -c:v libx264 -pix_fmt yuv420p -c:a aac -shortest -movflags +faststart clip.mp4
# The workspace directory must not exist or must be empty. Every step that the released version cannot
# do (a command it lacks, an operation it does not know) is recorded as "skipped" in <workspace>/scenario.txt,
# one line per step: "<step> ok" or "<step> skipped <reason>". TestUpgradeMatrix reads that file.
set -u

bin=$1
ws=$2
media=$3

mkdir -p "$ws"
# A released binary reads the variable of its own time: VUKEM_VIDEO_HOME before v0.30.0, STUARTY_HOME after.
export VUKEM_VIDEO_HOME="$ws" STUARTY_HOME="$ws"
log="$ws/scenario.txt"
: >"$log"

has_command() { "$bin" 2>&1 | grep -q "^  $1 "; }
step() { echo "$1 ok" >>"$log"; }
skip() { echo "$1 skipped $2" >>"$log"; }
run() { "$bin" "$@" >>"$ws/scenario.out" 2>&1; }

# From v0.50.0 on, project:create requires a description and a SMART goal; an older release refuses those
# flags, so they are sent only when the help of project:create lists them. The due date is far in the
# future because the release refuses a date in the past.
goal=()
if "$bin" project:create --help 2>&1 | grep -q -- '--description'; then
	goal=(
		--description "A project made by the upgrade test, to prove that a workspace of this release opens with the candidate."
		--specific "Open this workspace with the candidate build"
		--measurable "Every project keeps its state after the upgrade"
		--achievable "The scenario is small and runs in seconds"
		--relevant "It protects the work of a user who upgrades"
		--time-bound "Done within the run of the upgrade test"
		--due 2999-12-31
	)
fi
# create <id> <flags>: project:create with the goal flags this release needs (bash 3.2 safe on an empty
# array). From v0.50.0 on a new project has no timeline (the size is asked when a video is added), and the
# edits and TestUpgradeMatrix read the document of its timeline: when the release shows no document for the
# new project and has item:add, the scenario adds the timeline a user of that release would add.
create() {
	local id=$1
	shift
	run project:create --id "$id" "$@" ${goal[@]+"${goal[@]}"} || return 1
	if ! "$bin" project:show "$id" --detail document --json >/dev/null 2>&1 && has_command item:add; then
		run item:add --project "$id" --kind timeline --title "Main cut" --preset 9:16 --fps 30 --json || return 1
	fi
}

main=prj_1000000001
tracks_video=trk_1000000001
tracks_caption=trk_1000000002
tracks_title=trk_1000000003
asset=ast_1000000001
clip=itm_1000000001

create "$main" --name "Upgrade main" --preset 9:16 --fps 30 --json || { skip project.create "project:create failed"; exit 1; }
step project.create

if run media:import "$media" --project "$main" --asset-id "$asset" --wait --json; then step media.import; else skip media.import "import failed"; fi

# batch <step> <skip reason> <ops json>: one atomic edit, so each step stands alone.
batch() {
	printf '%s\n' "$3" >"$ws/ops-$1.json"
	if run edit --project "${proj:-$main}" --ops "$ws/ops-$1.json" --json; then step "$1"; else skip "$1" "$2"; fi
}

batch edit.captions "edit failed" "[
 {\"op\":\"track.add\",\"args\":{\"kind\":\"visual\",\"name\":\"Video\",\"id\":\"$tracks_video\"}},
 {\"op\":\"track.add\",\"args\":{\"kind\":\"visual\",\"name\":\"Captions\",\"id\":\"$tracks_caption\"}},
 {\"op\":\"track.add\",\"args\":{\"kind\":\"visual\",\"name\":\"Titles\",\"id\":\"$tracks_title\"}},
 {\"op\":\"caption.add\",\"args\":{\"track_id\":\"$tracks_caption\",\"start\":\"0s\",\"text\":\"Upgrade me safely\",\"duration\":\"2s\",\"linked_item\":\"$clip\",\"id\":\"itm_1000000002\"}},
 {\"op\":\"text.add\",\"args\":{\"track_id\":\"$tracks_title\",\"start\":\"0s\",\"duration\":\"1s\",\"text\":\"Title\",\"id\":\"itm_1000000003\"}}
]"

batch edit.clip "the media import is not available in this version" "[
 {\"op\":\"clip.add\",\"args\":{\"track_id\":\"$tracks_video\",\"asset_id\":\"$asset\",\"start\":\"0s\",\"id\":\"$clip\"}}
]"

batch edit.keyframes "the operation keyframe.set is not in this version, or there is no clip" "[
 {\"op\":\"keyframe.set\",\"args\":{\"item_id\":\"$clip\",\"property\":\"scale\",\"at\":\"0s\",\"value\":1,\"easing\":\"linear\"}},
 {\"op\":\"keyframe.set\",\"args\":{\"item_id\":\"$clip\",\"property\":\"scale\",\"at\":\"1s\",\"value\":1.2,\"easing\":\"ease_out\"}}
]"

if has_command template:save; then
	# A template carries no media (E_SLOT), so it is made from a project of text only.
	create prj_1000000005 --name "Template source" --json
	proj=prj_1000000005 batch edit.template_source "failed" "[
 {\"op\":\"track.add\",\"args\":{\"kind\":\"visual\",\"name\":\"Text\",\"id\":\"trk_1000000004\"}},
 {\"op\":\"text.add\",\"args\":{\"track_id\":\"trk_1000000004\",\"start\":\"0s\",\"duration\":\"1s\",\"text\":\"Template\",\"id\":\"itm_1000000004\"}}
]"
	if run template:save --project prj_1000000005 --name "Upgrade template" --json; then step template.save; else skip template.save "failed"; fi
else
	skip template.save "no template commands in this version"
fi

if has_command template:list && has_command template:new; then
	printf "{\"author\":{\"text\":\"Ada\"},\"quote\":{\"text\":\"Keep it simple\"}}" >"$ws/fill.json"
	tpl=$("$bin" template:list --json 2>/dev/null | grep -o '"template_id": *"tpl_[a-z0-9]*"' | grep -o 'tpl_5tar[a-z0-9]*' | head -1)
	if [ -n "$tpl" ] && run template:new "$tpl" --name "From starter" --id prj_1000000002 --fill "$ws/fill.json" --json; then
		step template.instantiate
	else
		skip template.instantiate "no starter template could be instantiated"
	fi
else
	skip template.instantiate "no template commands in this version"
fi

create prj_1000000003 --name "Trash and restore" --json &&
	run project:trash prj_1000000003 --json &&
	run project:restore prj_1000000003 --json && step project.trash_restore ||
	skip project.trash_restore "failed"

create prj_1000000004 --name "Left in the trash" --json &&
	run project:trash prj_1000000004 --json && step project.trashed || skip project.trashed "failed"

if run export --project "$main" --wait --json; then step export; else skip export "export failed"; fi
