#!/usr/bin/env bash
# H1: capability probe of the LGPL ffmpeg ($LGPL_FFMPEG).
: "${LGPL_FFMPEG:?set LGPL_FFMPEG}"
F="$LGPL_FFMPEG"; W="${WORK:-./work}"; mkdir -p "$W" logs
{ $F -hide_banner -version; } > logs/h1_version_lgpl.txt 2>&1
ffmpeg -hide_banner -version > logs/h1_version_path_gpl.txt 2>&1
grep -oE -- '--enable-(gpl|nonfree)' logs/h1_version_lgpl.txt || echo "no --enable-gpl / --enable-nonfree in LGPL configure line" > logs/h1_gpl_check.txt
$F -hide_banner -encoders > logs/h1_encoders.txt 2>&1
$F -hide_banner -filters > logs/h1_filters.txt 2>&1
: > logs/h1_filter_presence.txt
for f in overlay scale zscale tonemap xfade fps format setpts trim atrim amix volume afade loudnorm adelay concat pad crop transpose colorspace eq boxblur gblur fade alphamerge premultiply blend; do
  for b in lgpl path; do
    if [ $b = lgpl ]; then L=logs/h1_filters.txt; else ffmpeg -hide_banner -filters > "$W/pf.txt" 2>&1; L="$W/pf.txt"; fi
    n=$(grep -cE "^ [A-Z.]{2,3} +$f +" "$L"); echo "$f $b $n" >> logs/h1_filter_presence.txt
  done
done
: > logs/h1_encoder_trials.txt
for e in h264_mf libopenh264 h264_nvenc h264_qsv h264_amf h264_d3d12va h264_vulkan h264_vaapi; do
  echo "== $e" >> logs/h1_encoder_trials.txt
  $F -hide_banner -v error -y -f lavfi -i testsrc2=s=1080x1920:r=30:d=2 -vf format=yuv420p -c:v $e -b:v 8M "$W/h1_$e.mp4" >> logs/h1_encoder_trials.txt 2>&1 && echo "OK exit=0 size=$(stat -c %s "$W/h1_$e.mp4")" >> logs/h1_encoder_trials.txt || echo "FAILED exit=$?" >> logs/h1_encoder_trials.txt
done
for e in aac aac_mf; do
  echo "== audio $e" >> logs/h1_encoder_trials.txt
  $F -hide_banner -y -f lavfi -i sine=d=2 -c:a $e -b:a 192k "$W/h1_$e.m4a" 2>&1 | tail -3 >> logs/h1_encoder_trials.txt
done
