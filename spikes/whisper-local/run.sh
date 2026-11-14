#!/usr/bin/env bash
# Reproduces the whisper-local spike measurements (Windows 11, Git Bash, ffmpeg on PATH, gh, curl, python).
# Binaries, models and audio live in $SCRATCH, never in the repo. Small text results go to ./results.
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRATCH="${SCRATCH:-$WORK/}"
REL="${REL:-v1.9.2}"
MODELS="${MODELS:-tiny base small large-v3-turbo}"
LONG_MODELS="${LONG_MODELS:-base small large-v3-turbo}"
BIN="$SCRATCH/bin/Release/whisper-cli.exe"
RES="$HERE/results"
mkdir -p "$SCRATCH"/{dl,bin,models,audio,out} "$RES"

now_ms() { echo $(( $(date +%s%N) / 1000000 )); }

setup() {
	# v1.9.5 (latest stable, 2026-10-06) ships no binaries; v1.9.2 is the newest stable release with Windows assets.
	[ -f "$BIN" ] || { gh release download "$REL" -R ggml-org/whisper.cpp -p whisper-bin-x64.zip -D "$SCRATCH/dl" --clobber \
		&& unzip -q -o "$SCRATCH/dl/whisper-bin-x64.zip" -d "$SCRATCH/bin"; }
	for m in $MODELS; do
		[ -f "$SCRATCH/models/ggml-$m.bin" ] || curl -sL -o "$SCRATCH/models/ggml-$m.bin" \
			"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-$m.bin"
	done
	A="$SCRATCH/audio"
	# LibriVox recordings (public domain): O Alienista ch.1 (pt, reader Leni), Alice in Wonderland ch.1 (en, Kristen McQuillin)
	[ -f "$A/alien1.mp3" ] || curl -sL -o "$A/alien1.mp3" https://www.archive.org/download/o_alienista_0904_librivox/oalienista_1_machadodeassis_64kb.mp3
	[ -f "$A/alice1.mp3" ] || curl -sL -o "$A/alice1.mp3" https://www.archive.org/download/alice_in_wonderland_librivox/wonderland_ch_01_64kb.mp3
	[ -f "$A/papeis.txt" ] || curl -sL -o "$A/papeis.txt" https://www.gutenberg.org/cache/epub/57001/pg57001.txt
	[ -f "$A/alice.txt" ] || curl -sL -o "$A/alice.txt" https://www.gutenberg.org/cache/epub/11/pg11.txt
	ffmpeg -v error -y -ss 25 -t 90  -i "$A/alien1.mp3" -ar 16000 -ac 1 "$A/pt90.wav"
	ffmpeg -v error -y -ss 25 -t 90  -i "$A/alice1.mp3" -ar 16000 -ac 1 "$A/en90.wav"
	ffmpeg -v error -y -ss 25 -t 300 -i "$A/alice1.mp3" -ar 16000 -ac 1 "$A/en300.wav"
	for c in pt90 en90 en300; do
		ffmpeg -hide_banner -nostats -i "$A/$c.wav" -af silencedetect=noise=-35dB:d=0.3 -f null - 2> "$SCRATCH/out/$c.silence.log"
	done
}

env_info() {
	{
		powershell -NoProfile -c "Get-CimInstance Win32_Processor | select Name,NumberOfCores,NumberOfLogicalProcessors | fl; \
			'RAM bytes: ' + (Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory"
		"$BIN" --version 2>&1 | tail -1
		ls -l "$SCRATCH"/models "$SCRATCH"/bin/Release/whisper-cli.exe
	} > "$RES/environment.txt"
}

# run_one <model> <clip> <lang> <threads> <tag> [extra args...]; appends one TSV line to speed.tsv
run_one() {
	local m=$1 clip=$2 lang=$3 thr=$4 tag=$5; shift 5
	local of="$SCRATCH/out/$tag-$m-$clip-t$thr" dur t0 t1
	dur=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$SCRATCH/audio/$clip.wav")
	t0=$(now_ms)
	"$BIN" -m "$SCRATCH/models/ggml-$m.bin" -f "$SCRATCH/audio/$clip.wav" -l "$lang" -t "$thr" -ml 1 -sow -ojf -pp -of "$of" "$@" \
		> "$of.stdout" 2> "$of.stderr"
	local rc=$? ; t1=$(now_ms)
	python -I -c "import sys;w=($t1-$t0)/1000;d=float('$dur');print('\t'.join(['$tag','$m','$clip','$thr',str(round(d,1)),str(round(w,2)),str(round(d/w,2)),'$rc']))" >> "$RES/speed.tsv"
}

speed() {
	[ -s "$RES/speed.tsv" ] || printf 'tag\tmodel\tclip\tthreads\taudio_s\twall_s\trealtime_x\texit\n' > "$RES/speed.tsv"
	for m in $MODELS; do
		for c in pt90:pt en90:en; do
			for t in 4 8 16; do run_one "$m" "${c%%:*}" "${c##*:}" "$t" speed; done
		done
	done
	# 5-minute clip (long-form scaling)
	for m in $LONG_MODELS; do run_one "$m" en300 en 8 long; done
}

timing_quality() {
	for m in $MODELS; do
		for c in pt90:pt en90:en; do
			n=${c%%:*}
			f="$SCRATCH/out/speed-$m-$n-t8.json"
			echo "{\"model\":\"$m\",\"clip\":\"$n\",\"mode\":\"-ml 1 -sow\",\"r\":$(python -I "$HERE/analyze.py" "$f" "$SCRATCH/out/$n.silence.log")}" >> "$RES/word-timing.jsonl"
		done
	done
	# DTW token timestamps (preset names: tiny base small large.v3.turbo)
	for m in $MODELS; do
		dtw=${m/-v3-turbo/.v3.turbo}
		for c in pt90:pt en90:en; do
			n=${c%%:*}; l=${c##*:}
			run_one "$m" "$n" "$l" 8 dtw -dtw "$dtw" -nfa
			echo "{\"model\":\"$m\",\"clip\":\"$n\",\"mode\":\"-dtw $dtw -nfa\",\"r\":$(python -I "$HERE/analyze.py" "$SCRATCH/out/dtw-$m-$n-t8.json" "$SCRATCH/out/$n.silence.log" dtw)}" >> "$RES/word-timing.jsonl"
		done
	done
}

contract() {
	{
		echo "## detect language (auto) on pt90, per model"
		for m in $MODELS; do
			echo "# $m"; "$BIN" -m "$SCRATCH/models/ggml-$m.bin" -f "$SCRATCH/audio/pt90.wav" -l auto -dl 2>&1 | grep -i "auto-detected"
		done
		echo "## missing file"
		"$BIN" -m "$SCRATCH/models/ggml-tiny.bin" -f "$SCRATCH/audio/nope.wav" 2>&1 | tail -3; echo "exit=${PIPESTATUS[0]}"
		echo "## corrupt file"
		head -c 5000 /dev/urandom > "$SCRATCH/audio/corrupt.wav"
		"$BIN" -m "$SCRATCH/models/ggml-tiny.bin" -f "$SCRATCH/audio/corrupt.wav" 2>&1 | tail -3; echo "exit=${PIPESTATUS[0]}"
		echo "## missing model"
		"$BIN" -m "$SCRATCH/models/nope.bin" -f "$SCRATCH/audio/pt90.wav" 2>&1 | tail -3; echo "exit=${PIPESTATUS[0]}"
		echo "## progress lines (stderr, -pp), tiny pt90"
		grep -a progress "$SCRATCH/out/speed-tiny-pt90-t4.stderr" | head -12
		echo "## stdout head"
		head -5 "$SCRATCH/out/speed-tiny-pt90-t4.stdout"
	} > "$RES/contract.txt" 2>&1
}

accuracy() {
	: > "$RES/accuracy.jsonl"
	for m in $MODELS; do
		for c in pt90:"As chronicas da villa":papeis en90:"but it had no pictures":alice; do
			IFS=: read -r n phrase ref <<< "$c"
			echo "{\"model\":\"$m\",\"clip\":\"$n\",\"r\":$(python -I "$HERE/wer40.py" "$SCRATCH/out/speed-$m-$n-t8.json" "$SCRATCH/audio/$ref.txt" "$phrase")}" >> "$RES/accuracy.jsonl"
		done
	done
}

case "${1:-all}" in
	accuracy) accuracy ;;
	env) env_info ;;
	setup) setup ;;
	speed) speed ;;
	timing) timing_quality ;;
	contract) contract ;;
	all) setup && env_info && speed && timing_quality && accuracy && contract ;;
esac
