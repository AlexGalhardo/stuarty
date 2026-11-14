#!/usr/bin/env bash
# Reproduces the spike. Requires: go, bash, gyan-style GPL ffmpeg on PATH (reference only),
# and the BtbN LGPL build: export LGPL_FFMPEG=/path/to/ffmpeg-n9.0-latest-win64-lgpl-9.0/bin/ffmpeg.exe
# Optional: WORK=/path/to/scratch (large files; default ./work, git-ignored).
set -euo pipefail
: "${LGPL_FFMPEG:?set LGPL_FFMPEG}"
HERE="$(cd "$(dirname "$0")" && pwd)"; cd "$HERE"
export WORK="${WORK:-$HERE/work}"; mkdir -p "$WORK" logs results
LP="${LGPL_FFMPEG%ffmpeg*}ffprobe.exe"; [ -x "$LP" ] || LP="$(dirname "$LGPL_FFMPEG")/ffprobe"
go build -o "$WORK/exporter" .
X="$WORK/exporter"
ENCODERS="${ENCODERS:-h264_mf libopenh264 h264_nvenc}"   # H1 working encoders on the test machine

echo "### H1"; bash h1.sh

echo "### source"; "$X" gen-source -ffmpeg ffmpeg -out "$WORK/source.mp4"

echo "### H2"; : > results/h2.jsonl
for run in 1 2; do for s in s1 s2 s3; do
  for e in $ENCODERS libx264; do
    FF="$LGPL_FFMPEG"; [ $e = libx264 ] && FF=ffmpeg   # libx264 = GPL reference only
    "$X" export -ffmpeg "$FF" -ffprobe "$LP" -src "$WORK/source.mp4" -out "$WORK/h2_${s}_${e}.mp4" \
      -strategy $s -encoder $e -run $run -work "$WORK" -results results/h2.jsonl
  done
done; done

echo "### H3"
for n in a b; do
  "$X" export -ffmpeg "$LGPL_FFMPEG" -src "$WORK/source.mp4" -out "$WORK/md5_lgpl_$n.txt" -strategy s1 -mode framemd5 -run 1 -results results/h3.jsonl
done
"$X" export -ffmpeg ffmpeg -src "$WORK/source.mp4" -out "$WORK/md5_gyan.txt" -strategy s1 -mode framemd5 -results results/h3.jsonl
"$X" export -ffmpeg "$LGPL_FFMPEG" -src "$WORK/source.mp4" -out "$WORK/md5_lgpl_s2.txt" -strategy s2 -mode framemd5 -results results/h3.jsonl
"$X" export -ffmpeg "$LGPL_FFMPEG" -src "$WORK/source.mp4" -out "$WORK/md5_lgpl_s3.txt" -strategy s3 -mode framemd5 -work "$WORK" -results results/h3.jsonl
cmp3() { # count differing frames (hash column) between two framemd5 files
  paste -d'|' <(grep -v '^#' "$1" | awk -F', *' '{print $6}') <(grep -v '^#' "$2" | awk -F', *' '{print $6}') \
    | awk -F'|' -v n="$3" '{t++; if($1!=$2)d++} END{printf "%s: frames=%d differing=%d\n", n, t, d}'
}
{ cmp3 "$WORK/md5_lgpl_a.txt" "$WORK/md5_lgpl_b.txt" "lgpl run1 vs lgpl run2 (S1)"
  cmp3 "$WORK/md5_lgpl_a.txt" "$WORK/md5_gyan.txt"   "lgpl vs gyan (S1)"
  cmp3 "$WORK/md5_lgpl_a.txt" "$WORK/md5_lgpl_s2.txt" "S1 vs S2 (lgpl)"
  cmp3 "$WORK/md5_lgpl_a.txt" "$WORK/md5_lgpl_s3.txt" "S1 vs S3 (lgpl)"; } | tee logs/h3_compare.txt
cp "$WORK/md5_lgpl_a.txt" logs/h3_framemd5_lgpl_s1.txt

echo "### H4"
"$X" export -ffmpeg "$LGPL_FFMPEG" -src "$WORK/source.mp4" -out "$WORK/ref.mkv" -strategy s1 -encoder ffv1
: > logs/h4_quality.txt
for e in $ENCODERS libx264; do
  FF="$LGPL_FFMPEG"; [ $e = libx264 ] && FF=ffmpeg
  extra=""; [ $e = libx264 ] && extra="-preset veryfast"
  $FF -hide_banner -loglevel error -y -i "$WORK/ref.mkv" -c:v $e -b:v 8M -g 60 -pix_fmt yuv420p $extra "$WORK/h4_$e.mp4"
  sz=$(stat -c %s "$WORK/h4_$e.mp4")
  ss=$("$LGPL_FFMPEG" -hide_banner -i "$WORK/h4_$e.mp4" -i "$WORK/ref.mkv" -lavfi ssim -f null - 2>&1 | grep -o 'SSIM Y.*')
  ps=$("$LGPL_FFMPEG" -hide_banner -i "$WORK/h4_$e.mp4" -i "$WORK/ref.mkv" -lavfi psnr -f null - 2>&1 | grep -o 'PSNR y.*')
  echo "$e size=$sz bitrate_kbps=$((sz*8/30/1000)) | $ss | $ps" | tee -a logs/h4_quality.txt
done
