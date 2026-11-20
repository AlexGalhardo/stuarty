package export

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// FFmpegWebP is the WebPEncoder of the product: FFmpeg with libwebp, a separate process started through the
// argument grammar (ADR-0003, SDD-007 section 6). The picture goes in as a PNG file of a run directory and the
// WebP comes out as a file of the same directory, which is removed afterwards.
type FFmpegWebP struct {
	Runner *ffmpeg.Runner
	// MaxBytes arms the size watchdog of the run; 0 leaves it off.
	MaxBytes int64
}

// maxWebPBytes is the largest WebP that is read back into memory: a 8192 x 8192 picture is far below it.
const maxWebPBytes = 512 << 20

// EncodeWebP encodes the PNG as a WebP. Quality 0 is lossless; 1 to 100 is lossy. A build of FFmpeg with no
// libwebp encoder answers E_EXPORT_FAILED with reason webp_unavailable.
func (e FFmpegWebP) EncodeWebP(ctx context.Context, png []byte, quality int) ([]byte, error) {
	if e.Runner == nil {
		return nil, webpUnavailable(errors.New("no ffmpeg runner"))
	}

	var id [6]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, exportErr(ReasonEncoder, "The image could not be encoded.", err, "")
	}

	dir, err := e.Runner.NewRunDir("webp-" + hex.EncodeToString(id[:]))
	if err != nil {
		return nil, wrapRun(err, ReasonEncoder, "The image could not be encoded.", "")
	}

	defer func() { _ = dir.Remove() }()

	in, err := dir.Path("in.png")
	if err != nil {
		return nil, exportErr(ReasonEncoder, "The image could not be encoded.", err, "")
	}

	out, err := dir.Path("out.webp")
	if err != nil {
		return nil, exportErr(ReasonEncoder, "The image could not be encoded.", err, "")
	}

	if err := os.WriteFile(in, png, 0o600); err != nil {
		return nil, exportErr(ReasonEncoder, "The image could not be encoded.", err, "")
	}

	argv, err := ffargs.WebPStill(e.Runner.Policy(dir), in, out, quality)
	if err != nil {
		return nil, exportErr(ReasonEncoder, "The image could not be encoded.", err, "")
	}

	res, err := e.Runner.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: dir, Argv: argv, Deadline: ffmpeg.ExportDeadline(0), DeadlineCeiling: ffmpeg.ExportDeadlineCeiling,
		OutputPath: out, MaxOutputBytes: e.MaxBytes,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if missingWebPEncoder(res.Stderr) {
			return nil, webpUnavailable(err)
		}

		return nil, wrapRun(err, ReasonEncoder, "The image could not be encoded as WebP.", res.Stderr)
	}

	fi, err := os.Stat(out)
	if err != nil || fi.Size() == 0 {
		return nil, exportErr(ReasonEncoder, "FFmpeg wrote no WebP file.", errors.New("no output"), res.Stderr)
	}

	// The watchdog polls, so a file can finish past the limit between two polls: it is measured again here,
	// before it is read into memory.
	if limit := e.readLimit(); fi.Size() > limit {
		return nil, sizeLimitError(fi.Size(), limit)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		return nil, exportErr(ReasonEncoder, "The WebP file could not be read.", err, "")
	}

	return b, nil
}

// missingWebPEncoder tells the failure of a build with no libwebp from any other.
func missingWebPEncoder(stderr string) bool {
	s := strings.ToLower(stderr)

	return strings.Contains(s, "unknown encoder") || strings.Contains(s, "encoder 'libwebp' not found") ||
		strings.Contains(s, "codec not found")
}

// readLimit is the largest WebP read back: STUARTY_MAX_EXPORT_BYTES when it is set and lower than maxWebPBytes.
func (e FFmpegWebP) readLimit() int64 {
	if e.MaxBytes > 0 && e.MaxBytes < maxWebPBytes {
		return e.MaxBytes
	}

	return maxWebPBytes
}

func webpUnavailable(cause error) *ffmpeg.Error {
	e := exportErr(ReasonWebPUnavailable, "WebP export is not available on this machine.", cause, "")
	e.Hint = "Export PNG or JPEG, or use an FFmpeg build with the libwebp encoder."

	return e
}
