#!/usr/bin/env bash
# Run a packed whisper-cli on 20 seconds of real speech with the tiny model and
# require at least 30 words (the integration test of SDD-011 A-18):
#   bash scripts/whisper-test.sh <archive> <work-dir>
# The archive is the output of scripts/whisper-build.sh. The model and the audio
# are the pins of .config/whisper.env; the model is checked by SHA-256.
set -euo pipefail
archive="${1:?usage: whisper-test.sh <archive> <work-dir>}"
work="${2:?usage: whisper-test.sh <archive> <work-dir>}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=../.config/whisper.env
. "$root/.config/whisper.env"

mkdir -p "$work"
work="$(cd "$work" && pwd)"

case "$archive" in
	*.zip) powershell -NoProfile -Command "Expand-Archive -Force -Path \"$(cygpath -w "$archive")\" -DestinationPath \"$(cygpath -w "$work")\"" ;;
	*) tar -xzf "$archive" -C "$work" ;;
esac

exe="$(find "$work" -type f \( -name whisper-cli -o -name whisper-cli.exe \) | head -n 1)"
test -n "$exe" || { echo "whisper-test: no whisper-cli in $archive" >&2; exit 1; }
chmod +x "$exe"

curl -fsSL -o "$work/ggml-tiny.bin" "$WHISPER_TEST_MODEL_URL"
echo "${WHISPER_TEST_MODEL_SHA256}  $work/ggml-tiny.bin" | sha256sum -c - >&2
curl -fsSL -o "$work/speech.mp3" "$WHISPER_TEST_AUDIO_URL"

"$exe" --version >&2 || true
"$exe" -m "$work/ggml-tiny.bin" -f "$work/speech.mp3" -d 20000 -l en -nt > "$work/out.txt"

words="$(wc -w < "$work/out.txt" | tr -d " ")"
echo "whisper-test: $words words" >&2
head -c 400 "$work/out.txt" >&2
[ "$words" -ge 30 ] || { echo "whisper-test: fewer than 30 words" >&2; exit 1; }
