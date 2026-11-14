#!/usr/bin/env bash
# Reproduces the go-compositor spike.
# Requires: go, bash, an LGPL ffmpeg/ffprobe (export LGPL_FFMPEG=/path/to/ffmpeg.exe), and ffmpeg with libx264 on PATH
# only to generate the two 30 s fixtures. WORK = scratch dir for large files.
set -uo pipefail
: "${LGPL_FFMPEG:?set LGPL_FFMPEG}"
HERE="$(cd "$(dirname "$0")" && pwd)"; cd "$HERE"
WORK="${WORK:-$HERE/work}"; mkdir -p "$WORK" results logs
LP="${LGPL_FFMPEG%ffmpeg*}ffprobe.exe"
go build -o "$WORK/compositor.exe" . || exit 1
X="$WORK/compositor.exe"
[ -f "$WORK/portrait.mp4" ]  || "$X" gen-source -ffmpeg ffmpeg -kind portrait  -out "$WORK/portrait.mp4"
[ -f "$WORK/landscape.mp4" ] || "$X" gen-source -ffmpeg ffmpeg -kind landscape -out "$WORK/landscape.mp4"
R=results/runs.jsonl; : > "$R"
common=(-ffmpeg "$LGPL_FFMPEG" -ffprobe "$LP" -src "$WORK/portrait.mp4" -srcl "$WORK/landscape.mp4" -results "$R")
b() { "$X" run "${common[@]}" "$@"; }   # one measured run

for run in 1 2; do
  for enc in h264_nvenc libopenh264; do
    for c in c1 c2 c3 c5; do b -case $c -encoder $enc -out "$WORK/o.mp4" -run $run; done
    b -case c4 -encoder $enc -out "$WORK/o.mp4" -run $run                      # BiLinear
    b -case c4 -approx -encoder $enc -out "$WORK/o.mp4" -run $run              # ApproxBiLinear
    for c in c1 c3 c4 c5; do b -case $c -bands 8 -encoder $enc -out "$WORK/o.mp4" -run $run; done
    b -case c4 -approx -bands 8 -encoder $enc -out "$WORK/o.mp4" -run $run
    b -case c1 -allsws -encoder $enc -out "$WORK/o.mp4" -run $run              # -sws_flags on the C1 decoder
  done
  b -case c1 -encoder null -run $run
  b -case c1 -encoder nullconv -run $run
  b -case c1 -bands 8 -encoder null -run $run
  b -case c1 -allsws -encoder null -run $run
done

# determinism: per-frame SHA-256 of the composed RGBA frames, two runs each (null encoder: encoding is irrelevant)
for run in 1 2; do
  b -case c1 -encoder null -hashes "$WORK/h_c1_$run.txt" -run $run
  b -case c1 -bands 8 -encoder null -hashes "$WORK/h_c1b_$run.txt" -run $run
  b -case c4 -encoder null -hashes "$WORK/h_c4_$run.txt" -run $run
  b -case c4 -approx -encoder null -hashes "$WORK/h_c4a_$run.txt" -run $run
  b -case c4 -bands 8 -encoder null -hashes "$WORK/h_c4b_$run.txt" -run $run
done
{ "$X" cmp "$WORK/h_c1_1.txt" "$WORK/h_c1_2.txt"
  "$X" cmp "$WORK/h_c4_1.txt" "$WORK/h_c4_2.txt"
  "$X" cmp "$WORK/h_c4a_1.txt" "$WORK/h_c4a_2.txt"
  "$X" cmp "$WORK/h_c1_1.txt" "$WORK/h_c1b_1.txt"
  "$X" cmp "$WORK/h_c4_1.txt" "$WORK/h_c4b_1.txt"; } | tee logs/determinism.txt
