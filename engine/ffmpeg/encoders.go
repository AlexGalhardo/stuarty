package ffmpeg

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// EncoderOrder is ADR-0011 item 5: the first H.264 encoder that works on the machine wins.
// libx264 is last and only for the user's own FFmpeg when the user enabled it.
var EncoderOrder = []string{
	"h264_videotoolbox", "h264_nvenc", "h264_qsv", "h264_amf", "h264_mf", "h264_vaapi", "libopenh264", "libx264",
}

// EncoderStatus is the result of one candidate (SDD-011 8.4).
type EncoderStatus string

// The results of one candidate.
const (
	EncoderOK        EncoderStatus = "ok"
	EncoderFailed    EncoderStatus = "failed"
	EncoderNotListed EncoderStatus = "not_listed"
)

// Candidate is one encoder tried by detection.
type Candidate struct {
	Name       string
	Status     EncoderStatus
	MS         int64
	StderrTail string // at most 300 characters, paths removed
	// Reason is a machine readable cause for a candidate that was not tried, such as
	// ReasonOpenH264NotInstalled; empty otherwise.
	Reason string
	// Input is what the export must use with this encoder (SDD-011 8.4, DS-13).
	Input ffargs.DetectInput
}

// EncoderDetection is the cached result of the test encodes.
type EncoderDetection struct {
	Chosen     string // empty when none works
	Forced     bool
	Candidates []Candidate
}

// ChosenCandidate returns the candidate record of the chosen encoder.
func (d *EncoderDetection) ChosenCandidate() (Candidate, bool) {
	for _, c := range d.Candidates {
		if c.Name == d.Chosen {
			return c, true
		}
	}
	return Candidate{}, false
}

// DetectOptions are the inputs of encoder detection.
type DetectOptions struct {
	// Forced is STUARTY_ENCODER: only that candidate is tested and it is never replaced.
	Forced string
	// AllowUserX264 is STUARTY_ALLOW_USER_X264; it matters only for a user's own build.
	AllowUserX264 bool
	// Candidates replaces the default order when non-nil. An empty non-nil list disables
	// every encoder, which is how tests simulate a machine with none.
	Candidates []string
	// Refresh ignores the cached result (doctor always re-runs, SDD-011 8.5).
	Refresh bool
}

func platformOK(name string) bool {
	if CodecOf(name) == CodecHEVC || CodecOf(name) == CodecAV1 {
		return platformOKCodec(name)
	}

	switch name {
	case "h264_videotoolbox":
		return runtime.GOOS == "darwin"
	case "h264_mf":
		return runtime.GOOS == "windows"
	case "h264_vaapi":
		return runtime.GOOS == "linux"
	}
	return true
}

func vaapiDevice() string {
	m, _ := filepath.Glob("/dev/dri/renderD*")
	sort.Strings(m)
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

// gate is the answer of encoderGate: skip is true when the candidate is not worth a test encode.
type gate struct {
	skip   bool
	status EncoderStatus
	why    string
	reason string
}

// encoderGate decides, without starting anything, whether a candidate can be tried at all: the
// encoder must be one the product knows, exist on this platform, be allowed and be in the build.
// libopenh264 in the own build also needs the Cisco library installed (D-017); without it the
// candidate fails with the install command, and no test encode is wasted on a certain failure.
func (r *Runner) encoderGate(name string, caps *Capabilities, opt DetectOptions) gate {
	switch {
	case IsRefusedEncoder(name):
		return gate{
			true,
			EncoderFailed,
			"a GPL or nonfree encoder, which Stuarty never uses (ADR-0011)",
			ReasonRefusedEncoder,
		}
	case CodecOf(name) == "":
		return gate{true, EncoderFailed, "not an encoder the product knows", ""}
	case !platformOK(name):
		return gate{true, EncoderNotListed, "not available on " + runtime.GOOS, ""}
	case name == "libx264" && (!r.In.UserBuild || !opt.AllowUserX264):
		return gate{
			true,
			EncoderNotListed,
			"only a user's own FFmpeg with STUARTY_ALLOW_USER_X264=true may use it",
			"",
		}
	case !caps.Encoders[name]:
		return gate{true, EncoderNotListed, "", ""}
	case name == "libopenh264" && r.In.Flavour().Name == FlavourOwn && !r.In.OpenH264.Installed:
		return gate{
			true,
			EncoderFailed,
			"the Cisco OpenH264 library is not installed; run " + OpenH264InstallCommand,
			openH264Reason(r.In.OpenH264),
		}
	}
	return gate{}
}

// DetectEncoder runs a real test encode for each candidate, in the order of ADR-0011 item 5,
// and caches the result for the life of the runner. No working encoder is E_NO_ENCODER and
// the message says how to fix it.
func (r *Runner) DetectEncoder(ctx context.Context, caps *Capabilities, opt DetectOptions) (*EncoderDetection, error) {
	r.encMu.Lock()
	defer r.encMu.Unlock()
	if r.enc != nil && !opt.Refresh {
		return r.enc, r.encErr
	}
	det, err := r.detectEncoder(ctx, caps, opt)
	if ctx.Err() == nil { // a cancelled detection is not cached
		r.enc, r.encErr = det, err
	}
	return det, err
}

func (r *Runner) detectEncoder(ctx context.Context, caps *Capabilities, opt DetectOptions) (*EncoderDetection, error) {
	names := EncoderOrder
	if opt.Candidates != nil {
		names = opt.Candidates
	}

	if opt.Forced != "" {
		names = []string{opt.Forced}
	}

	return r.detectNames(ctx, caps, names, opt, CodecH264)
}

// detectNames runs the test encode of each name, in order, and chooses the first that works. For H.264 a
// failure is noEncoderError; for another codec it is noCodecError.
func (r *Runner) detectNames(
	ctx context.Context,
	caps *Capabilities,
	names []string,
	opt DetectOptions,
	codec string,
) (*EncoderDetection, error) {
	det := &EncoderDetection{Forced: opt.Forced != ""}

	dir, err := r.newInternalRunDir("encoder-detection")
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Remove() }()

	for _, name := range names {
		c := Candidate{Name: name}
		// A forced or listed name of another codec (STUARTY_ENCODER=hevc_nvenc) is not an encoder of this one: it
		// would pass its own test encode and then write the wrong codec.
		if known := CodecOf(name); known != "" && known != codec {
			c.Status, c.StderrTail = EncoderFailed, "a "+known+" encoder, not a "+codec+" one"
		} else if g := r.encoderGate(name, caps, opt); g.skip {
			c.Status, c.StderrTail, c.Reason = g.status, g.why, g.reason
		} else if err := r.testEncoder(ctx, dir, &c); err != nil && ctx.Err() != nil {
			return nil, err
		}
		det.Candidates = append(det.Candidates, c)
	}
	for _, c := range det.Candidates {
		if c.Status == EncoderOK {
			det.Chosen = c.Name
			break
		}
	}
	if det.Chosen == "" {
		if codec != CodecH264 {
			return det, noCodecError(codec, det)
		}

		return det, noEncoderError(det)
	}
	return det, nil
}

// testEncoder is the two commands of SDD-011 8.4: the encode, then ffprobe on its output.
func (r *Runner) testEncoder(ctx context.Context, dir *RunDir, c *Candidate) error {
	start := time.Now()
	defer func() { c.MS = time.Since(start).Milliseconds() }()

	di := ffargs.DetectInput{Filter: "format=yuv420p"}
	if strings.HasSuffix(c.Name, "_vaapi") {
		dev := vaapiDevice()
		if dev == "" {
			c.Status, c.StderrTail = EncoderFailed, "no render device in /dev/dri"
			return nil
		}
		di = ffargs.DetectInput{VaapiDevice: dev, Filter: "format=nv12,hwupload"}
	}
	out, err := dir.Path("probe-" + c.Name + ".mp4")
	if err != nil {
		return err
	}
	argv, err := ffargs.EncoderTest(r.Policy(dir), c.Name, di, out)
	if err != nil {
		c.Status, c.StderrTail = EncoderFailed, "the test encode was refused: "+scrub(err.Error(), 200)
		return nil
	}
	res, err := r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: detectionTimeout})
	if err != nil {
		c.Status, c.StderrTail = EncoderFailed, firstFailureLine(res.Stderr, err)
		return err
	}
	pargv, err := ffargs.ProbeEncodedPackets(r.Policy(dir), out)
	if err != nil {
		c.Status, c.StderrTail = EncoderFailed, "the check was refused"
		return nil
	}
	pctx, cancel := context.WithTimeout(ctx, detectionTimeout)
	defer cancel()
	b, pres, err := r.RunFFprobe(pctx, dir, pargv, detectionTimeout)
	if err != nil {
		c.Status, c.StderrTail = EncoderFailed, firstFailureLine(pres.Stderr, err)
		return err
	}
	// csv=p=0 prints "h264,15": the codec and the packet count.
	want := CodecOf(c.Name) + ",15"
	if got := strings.TrimSpace(string(b)); got != want {
		c.Status = EncoderFailed
		c.StderrTail = "the test file has " + scrub(got, 40) + " instead of 15 " + CodecOf(c.Name) + " packets"

		return nil
	}
	c.Status = EncoderOK
	c.Input = di
	return nil
}

// firstFailureLine is the explanation of a failed run: the first non-empty line of stderr (FFmpeg
// prints the cause first and the consequence after it), or a line about the error.
func firstFailureLine(stderr string, err error) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	line := ""
	for _, l := range lines {
		if s := strings.TrimSpace(l); s != "" {
			line = s
			break
		}
	}
	if line == "" && err != nil {
		switch {
		case errors.Is(err, ErrDeadline):
			line = "took longer than " + detectionTimeout.String()
		default:
			line = "the test encode failed"
		}
	}
	return scrub(line, 300)
}

func noEncoderError(det *EncoderDetection) *Error {
	var parts []string
	details := make([]map[string]any, 0, len(det.Candidates))
	for _, c := range det.Candidates {
		switch c.Status {
		case EncoderNotListed:
			why := "not in this FFmpeg build"
			if c.StderrTail != "" {
				why = c.StderrTail
			}
			parts = append(parts, c.Name+" ("+why+")")
		default:
			first, _, _ := strings.Cut(c.StderrTail, "\n")
			parts = append(parts, c.Name+" (failed: "+scrub(first, 120)+")")
		}
		details = append(details, map[string]any{"name": c.Name, "status": string(c.Status), "ms": c.MS})
	}
	tried := "nothing (every encoder is disabled)"
	if len(parts) > 0 {
		tried = strings.Join(parts, ", ")
	}
	e := &Error{
		Code:    CodeNoEncoder,
		Reason:  "no_encoder",
		Message: "No H.264 encoder works on this machine. Tried: " + tried + ".",
		Hint:    "Use the FFmpeg that ships with Stuarty, which includes OpenH264, or point " + EnvFFmpeg + " at an FFmpeg that has libx264 and set STUARTY_ALLOW_USER_X264=true.",
		Details: map[string]any{"candidates": details, "forced": det.Forced},
	}
	for _, c := range det.Candidates {
		if c.Name == "libopenh264" && strings.HasPrefix(c.Reason, "openh264_") {
			e.Hint = "Run \"" + OpenH264InstallCommand + "\" to install Cisco's OpenH264 library for this FFmpeg build " +
				"(nothing downloads it automatically), or use the hardware encoder of this machine, or point " + EnvFFmpeg +
				" at an FFmpeg that has libx264 and set STUARTY_ALLOW_USER_X264=true."
			e.Details["reason"], e.Details["install_command"], e.Details["tool"] = c.Reason, OpenH264InstallCommand, OpenH264Tool
			e.Details["missing"] = []string{OpenH264Tool}
		}
	}
	return e
}
