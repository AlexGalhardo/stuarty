#!/usr/bin/env bash
# Makes the synthetic speaker used to measure and test the face helper (epic 70, task 2): a drawn face
# (skin, hair, eyes, mouth) that crosses a 1280x720 frame in 10 s at 10 fps, with a small vertical sway.
# It is authored here with FFmpeg expressions, so it carries no third-party licence and no real person.
#
# usage: make-face-fixture.sh <ffmpeg> <output.mp4>
# The face centre at time T seconds is x = 240 + 80*T, y = 300 + 40*sin(2*T), in the 1280x720 frame.
set -euo pipefail

ffmpeg_bin=${1:?the path of ffmpeg}
out=${2:?the output file}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

CX='(240+800*T/10)'
CY='(300+40*sin(T*2))'
E() { echo "lt(pow((X-($1))/($3)\,2)+pow((Y-($2))/($4)\,2)\,1)"; }

FACE=$(E "$CX" "$CY" 95 120)
HAIR=$(E "$CX" "$CY-45" 105 100)
LEYE=$(E "$CX-38" "$CY-18" 16 9)
REYE=$(E "$CX+38" "$CY-18" 16 9)
LPUP=$(E "$CX-38" "$CY-18" 7 7)
RPUP=$(E "$CX+38" "$CY-18" 7 7)
NOSE=$(E "$CX" "$CY+18" 12 20)
MOUTH=$(E "$CX" "$CY+62" 32 9)
BROWL=$(E "$CX-38" "$CY-40" 22 5)
BROWR=$(E "$CX+38" "$CY-40" 22 5)
BODY="lt(abs(X-($CX))\,190)*gt(Y\,($CY+110))"

# chan <skin> <shirt> <background> <pupil> <mouth> <nose> <hair>: one colour channel.
chan() {
	echo "if($LPUP+$RPUP\,$4\,if($LEYE+$REYE\,255\,if($MOUTH\,$5\,if($BROWL+$BROWR\,$4+20\,if($NOSE*$FACE\,$6\,if($HAIR*lt(Y\,$CY-30)\,$7\,if($FACE\,$1\,if($BODY\,$2\,$3))))))))"
}

printf "geq=r='%s':g='%s':b='%s'" \
	"$(chan 224 40 60 20 170 205 40)" "$(chan 172 60 62 20 60 150 30)" "$(chan 140 120 66 25 70 120 25)" >"$work/face.vf"

"$ffmpeg_bin" -v error -y -f lavfi -i "color=c=gray:s=1280x720:r=10:d=10,format=rgb24" -/vf "$work/face.vf" \
	-pix_fmt yuv420p -c:v libopenh264 "$out"
