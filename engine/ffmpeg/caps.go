package ffmpeg

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// Capabilities is the listing of SDD-011 8.3.
type Capabilities struct {
	Encoders, Decoders, Filters, Muxers map[string]bool
	// Optional capabilities, recorded and never required.
	Zscale, Tonemap, Bwdif, Yadif bool
	// HDROK is true when the HDR tone-mapping chain ran on a synthetic HLG source.
	HDROK bool
	// License is the classification of 8.2 with the rule that libx264 or libx265 among the
	// encoders implies gpl.
	License License
}

// Required items of SDD-011 8.3.
var (
	requiredEncoders = []string{"mpeg4", "libvpx-vp9", "aac", "mjpeg", "png", "pcm_s16le"}
	requiredFilters  = []string{
		"fps",
		"scale",
		"split",
		"format",
		"setpts",
		"setsar",
		"aresample",
		"aformat",
		"apad",
		"atrim",
		"adelay",
		"silencedetect",
		"ebur128",
		"asplit",
	}
	requiredMuxers = []string{"mov", "mp4", "wav", "image2", "null"}
)

// parseListing reads the names of an ffmpeg -encoders, -decoders, -filters or -muxers listing:
// the second column of every line after the rule of dashes ("------" for most listings, "--" for muxers).
func parseListing(out string) map[string]bool {
	names := map[string]bool{}
	seen := false
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if t := strings.TrimSpace(line); len(t) >= 2 && strings.Trim(t, "-") == "" {
			seen = true
			continue
		}
		if !seen {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] != "=" {
			names[f[1]] = true
		}
	}
	return names
}

func (r *Runner) list(ctx context.Context, dir *RunDir, flag string) (map[string]bool, error) {
	argv, err := ffargs.Info(flag)
	if err != nil {
		return nil, internalErr("capability listing refused", err)
	}
	var out bytes.Buffer
	if _, err := r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: 30 * time.Second, Stdout: &out}); err != nil {
		return nil, err
	}
	return parseListing(out.String()), nil
}

func missingErr(missing []string) *Error {
	sort.Strings(missing)
	return &Error{
		Code:    CodeFFmpegMissing,
		Reason:  "capabilities_missing",
		Message: "The FFmpeg in use lacks what the product needs: " + strings.Join(missing, ", ") + ".",
		Hint:    `Use the FFmpeg that ships with Stuarty, or an FFmpeg 7 or newer that has these items. Run "stuarty doctor" for details.`,
		Details: map[string]any{"missing": missing},
	}
}

// missingRequired lists the required items that a listing lacks.
func missingRequired(c *Capabilities) []string {
	var missing []string
	need := func(kind string, set map[string]bool, names []string) {
		for _, n := range names {
			if !set[n] {
				missing = append(missing, kind+":"+n)
			}
		}
	}
	need("encoder", c.Encoders, requiredEncoders)
	need("filter", c.Filters, requiredFilters)
	need("muxer", c.Muxers, requiredMuxers)
	return missing
}

// ReadCapabilities parses the four listings, requires what SDD-011 8.3 requires, runs the
// smoke encodes of the required encoders and the HDR chain. A missing or failing required
// item is E_FFMPEG_MISSING with details.missing.
func (r *Runner) ReadCapabilities(ctx context.Context) (*Capabilities, error) {
	dir, err := r.newInternalRunDir("capabilities")
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Remove() }()

	c := &Capabilities{}
	for _, l := range []struct {
		flag string
		into *map[string]bool
	}{{"-encoders", &c.Encoders}, {"-decoders", &c.Decoders}, {"-filters", &c.Filters}, {"-muxers", &c.Muxers}} {
		if *l.into, err = r.list(ctx, dir, l.flag); err != nil {
			return nil, err
		}
	}

	if missing := missingRequired(c); len(missing) > 0 {
		return nil, missingErr(missing)
	}

	c.Zscale, c.Tonemap, c.Bwdif, c.Yadif = c.Filters["zscale"], c.Filters["tonemap"], c.Filters["bwdif"], c.Filters["yadif"]
	c.License = r.In.Info.License
	if (c.Encoders["libx264"] || c.Encoders["libx265"]) && c.License.Name == "lgpl" {
		c.License.Name = "gpl"
	}

	var missing []string
	for _, enc := range requiredEncoders {
		argv, err := ffargs.EncoderSmoke(r.Policy(dir), enc)
		if err != nil {
			return nil, internalErr("smoke encode refused", err)
		}
		if _, err := r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: detectionTimeout}); err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			missing = append(missing, "encoder:"+enc+" (does not encode)")
		}
	}
	if len(missing) > 0 {
		return nil, missingErr(missing)
	}

	if c.Zscale && c.Tonemap {
		if argv, err := ffargs.HDRSmoke(r.Policy(dir)); err == nil {
			_, runErr := r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: detectionTimeout})
			c.HDROK = runErr == nil
		}
	}
	return c, nil
}

// optionalFilters are accepted by the argv checker and used only when the build has them: HDR tone
// mapping, the deinterlacers (yadif stands in for bwdif) and the VAAPI upload.
var optionalFilters = map[string]bool{"zscale": true, "tonemap": true, "bwdif": true, "yadif": true, "hwupload": true}

// MissingFilters lists the filters the argv checker accepts and the engine needs that a build does not
// have. A build that lacks one fails here instead of in the middle of an export (SDD-004, SDD-011).
func MissingFilters(filters map[string]bool) []string {
	var missing []string
	for _, n := range ffargs.FilterNames() {
		if !optionalFilters[n] && !filters[n] {
			missing = append(missing, n)
		}
	}
	return missing
}
