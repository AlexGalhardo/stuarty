// Package ffargs builds and checks every FFmpeg and ffprobe argument list the product
// starts (SDD-007 section 6, rules FF-01 to FF-16).
//
// The grammar is an allowlist. Builders turn typed values into an argv, and [Check]
// scans an argv token by token and fails closed on anything it does not know. The
// runner calls Check before every start, and the builders call it on their own output,
// so an argv that leaves this package has always been through the same scanner as a
// hostile one.
package ffargs

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Tool is the binary an argv is for.
type Tool int

// The two programs an argv can be for.
const (
	FFmpeg Tool = iota
	FFprobe
)

func (t Tool) String() string {
	if t == FFprobe {
		return "ffprobe"
	}
	return "ffmpeg"
}

// Policy tells the checker where the child may read and write.
type Policy struct {
	// StoreDir is the content store. An input under it must be <StoreDir>/<aa>/<sha256>
	// with a lowercase hexadecimal digest (FF-03).
	StoreDir string
	// RunDir is the per-job directory: the only place an output may be written, and a
	// place an input may be read from (intermediate files of the same job).
	RunDir string
	// InputDirs are further directories an input may come from, such as the artifacts
	// folder of the same asset. Names inside them are checked like generated names.
	InputDirs []string
	// MaxThreads is the CPU budget of the run (FF-16). Zero means a -threads or
	// -filter_threads option is refused.
	MaxThreads int
}

// ErrPolicy is wrapped by every [ViolationError].
var ErrPolicy = errors.New("ffargs: argument list outside the grammar")

// ViolationError names the rule an argv broke and the token that broke it.
type ViolationError struct {
	Rule  string // FF-01 to FF-16
	Index int    // position in argv, -1 when the problem is the list as a whole
	Token string // the offending token, cut to 80 characters
	Msg   string
}

func (v *ViolationError) Error() string {
	if v.Index < 0 {
		return fmt.Sprintf("%s: %s", v.Rule, v.Msg)
	}
	return fmt.Sprintf("%s: %s (argument %d %q)", v.Rule, v.Msg, v.Index, v.Token)
}

func (v *ViolationError) Unwrap() error { return ErrPolicy }

func violate(rule string, argv []string, i int, format string, a ...any) *ViolationError {
	tok := ""
	if i >= 0 && i < len(argv) {
		tok = argv[i]
		if len(tok) > 80 {
			tok = tok[:80] + "..."
		}
	}
	return &ViolationError{Rule: rule, Index: i, Token: tok, Msg: fmt.Sprintf(format, a...)}
}

// Check scans argv (without the program name) for tool against the grammar and policy.
// A nil result means the argv is inside the grammar; anything else is a [*ViolationError].
func Check(tool Tool, argv []string, pol Policy) error {
	switch tool {
	case FFmpeg:
		return checkFFmpeg(argv, pol)
	case FFprobe:
		return checkFFprobe(argv, pol)
	}
	return &ViolationError{Rule: "FF-01", Index: -1, Msg: "unknown tool"}
}

// MaxAlloc is the -max_alloc value of FF-01: 1 GiB for any single allocation.
const MaxAlloc = 1 << 30

// InputFormats is the -format_whitelist of FF-04: the demuxer names of the allowed
// containers and image formats. HLS, DASH, concat, ffconcat, tee, sdp, rtsp, lavfi,
// subtitle formats and playlists are absent. TestSecurity_T14 runs every supported
// sample and every hostile one against this list on the pinned build.
const InputFormats = "mov,mp4,m4a,3gp,3g2,mj2,matroska,webm,avi,mpegts,mpeg,flv,wav,mp3,ogg,flac,aac,image2,png_pipe,jpeg_pipe,webp_pipe,gif,apng,webp_anim"

// inputFormatSet is InputFormats as a set, for checking ffprobe's format_name.
var inputFormatSet = func() map[string]bool {
	m := map[string]bool{}
	for n := range strings.SplitSeq(InputFormats, ",") {
		m[n] = true
	}
	return m
}()

// AllowedFormatName reports whether every name of an ffprobe format_name is allowed (FF-06).
func AllowedFormatName(formatName string) bool {
	if formatName == "" {
		return false
	}
	for n := range strings.SplitSeq(formatName, ",") {
		if !inputFormatSet[n] {
			return false
		}
	}
	return true
}

var (
	hexDigest   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hexPair     = regexp.MustCompile(`^[0-9a-f]{2}$`)
	pathElement = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._%+-]{0,127}$`)
	seqPattern  = regexp.MustCompile(`%0[1-9]d`)
)

// pathKind says where a path argument is going.
type pathKind int

const (
	pathInput pathKind = iota
	pathOutput
	// pathRunInput is an input the engine wrote itself (the ffmetadata of the chapters, FF-08): only the
	// run directory, never the store or an artifact directory.
	pathRunInput
)

func toSlash(p string) string { return filepath.ToSlash(filepath.Clean(p)) }

func hasPrefixDir(path, root string) (string, bool) {
	if root == "" {
		return "", false
	}
	root = toSlash(root)
	if len(path) <= len(root)+1 || path[len(root)] != '/' {
		return "", false
	}
	head := path[:len(root)]
	if head != root && (runtime.GOOS != "windows" || !strings.EqualFold(head, root)) {
		return "", false
	}
	return path[len(root)+1:], true
}

func absolute(p string) bool {
	if strings.HasPrefix(p, "/") {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && p[2] == '/' && (p[0]|0x20 >= 'a' && p[0]|0x20 <= 'z')
}

// checkFilePath validates the part after "file:" of an input or output (FF-03, FF-14).
func checkFilePath(rule string, argv []string, i int, p string, kind pathKind, pol Policy) error {
	if strings.ContainsAny(p, "\\\x00\n\r") {
		return violate(rule, argv, i, "path has a backslash or a control character; use the slash form")
	}
	if !absolute(p) {
		return violate(rule, argv, i, "path is not absolute")
	}
	if p != toSlash(p) {
		return violate(rule, argv, i, "path is not clean")
	}

	check := func(rel string, store bool) error {
		parts := strings.Split(rel, "/")
		if store {
			if len(parts) != 2 || !hexPair.MatchString(parts[0]) || !hexDigest.MatchString(parts[1]) ||
				parts[0] != parts[1][:2] {
				return violate(
					rule,
					argv,
					i,
					"a store path is <store>/<aa>/<sha256> with a lowercase hexadecimal digest",
				)
			}
			return nil
		}
		for _, e := range parts {
			if !pathElement.MatchString(e) {
				return violate(rule, argv, i, "path element %q is not a generated name", e)
			}
		}
		name := parts[len(parts)-1]
		rest := seqPattern.ReplaceAllString(name, "")
		if strings.Contains(rest, "%") {
			return violate(rule, argv, i, "only a %%0Nd frame number may appear in a name")
		}
		if kind != pathOutput && strings.Contains(name, "%") {
			return violate(rule, argv, i, "an input name has no pattern (image2 sequences are not used)")
		}
		return nil
	}

	if kind == pathRunInput {
		rel, ok := hasPrefixDir(p, pol.RunDir)
		if !ok {
			return violate(rule, argv, i, "an ffmetadata input is a file of the run directory")
		}
		return check(rel, false)
	}
	if kind == pathOutput {
		rel, ok := hasPrefixDir(p, pol.RunDir)
		if !ok {
			return violate(rule, argv, i, "an output is written only inside the run directory")
		}
		return check(rel, false)
	}
	if rel, ok := hasPrefixDir(p, pol.StoreDir); ok {
		return check(rel, true)
	}
	if rel, ok := hasPrefixDir(p, pol.RunDir); ok {
		return check(rel, false)
	}
	for _, d := range pol.InputDirs {
		if rel, ok := hasPrefixDir(p, d); ok {
			return check(rel, false)
		}
	}
	return violate(
		rule,
		argv,
		i,
		"an input is read only from the store, the run directory or an allowed artifact directory",
	)
}

// splitFilePrefix returns the path of a "file:" argument.
func splitFilePrefix(s string) (string, bool) {
	if !strings.HasPrefix(s, "file:") {
		return "", false
	}
	return strings.TrimPrefix(s, "file:"), true
}
