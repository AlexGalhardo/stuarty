package ingest

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

type probedPacket struct {
	PTSTime string `json:"pts_time"`
	Size    string `json:"size"`
	Pos     string `json:"pos"`
	Flags   string `json:"flags"`
}

// ffprobePackets lists the packets of the first video stream of a produced file. The test runs the
// pinned ffprobe itself: the argv policy of the product refuses -show_packets, which is for untrusted input.
func ffprobePackets(t *testing.T, path string) []probedPacket {
	t.Helper()

	in, _ := pinnedInstall(t)
	out, err := exec.CommandContext(
		context.Background(), in.FFprobe.Path(),
		"-v", "error", "-select_streams", "v:0", "-show_packets", "-of", "json", path,
	).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}

	var doc struct {
		Packets []probedPacket `json:"packets"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}

	return doc.Packets
}

// The index of a proxy says, for each sample, what ffprobe says of each packet: where it is, how big
// it is, when it is shown and whether it is a key frame (TASK-0197). The clips cover a proxy with
// audio, a longer one at 24 fps with a key frame every half second, and a short landscape one.
func TestProxyChunkIndexMatchesFFprobe(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	portrait, perr := e.gen.Portrait(ctx)
	long, lerr := e.gen.Clip(ctx, "long.mp4", 640, 360, 24, 6)
	ntsc, nerr := e.gen.Clip(ctx, "ntsc.mp4", 320, 240, 30, 4)

	for name, path := range map[string]string{
		"portrait": mustGen(t, portrait, perr),
		"long":     mustGen(t, long, lerr),
		"short":    mustGen(t, ntsc, nerr),
	} {
		t.Run(name, func(t *testing.T) {
			hash, res := e.mustIngest(path)
			proxy := e.publishedPath(hash, res.find("proxy", ""))

			idx, err := ReadChunkIndex(proxy)
			if err != nil {
				t.Fatal(err)
			}

			pk := ffprobePackets(t, proxy)
			if len(pk) != len(idx.Sizes) || len(pk) == 0 {
				t.Fatalf("%d samples in the index, %d packets in ffprobe", len(idx.Sizes), len(pk))
			}

			keys := map[int]bool{}
			for _, k := range idx.Keyframes {
				keys[k] = true
			}

			for i, p := range pk {
				pos, _ := strconv.ParseInt(p.Pos, 10, 64)
				size, _ := strconv.ParseInt(p.Size, 10, 64)
				pts, _ := strconv.ParseFloat(p.PTSTime, 64)

				if idx.Offsets[i] != pos || idx.Sizes[i] != size {
					t.Fatalf("sample %d at %d+%d, ffprobe says %d+%d", i, idx.Offsets[i], idx.Sizes[i], pos, size)
				}

				if d := math.Abs(float64(idx.TimestampsUS[i]) - pts*1e6); d > 1 {
					t.Fatalf("sample %d at %d us, ffprobe says %s s", i, idx.TimestampsUS[i], p.PTSTime)
				}

				if want := strings.HasPrefix(p.Flags, "K"); keys[i] != want {
					t.Fatalf("sample %d key frame = %v, ffprobe flags %q", i, keys[i], p.Flags)
				}
			}

			if name == "long" && len(idx.Keyframes) < 10 {
				t.Errorf("the 6 s clip has %d key frames, want one every half second", len(idx.Keyframes))
			}

			if len(idx.Keyframes) != len(keys) {
				t.Errorf("duplicate key frames in %v", idx.Keyframes)
			}
		})
	}
}
