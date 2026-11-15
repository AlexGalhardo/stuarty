package ffargs

import (
	"regexp"
	"sort"
	"strings"
)

// Filters the checker accepts: the union of those SDD-004 and SDD-011 name (FF-10). A
// filter of the FF-10 ceiling that neither names stays out, and the entries of
// forbiddenFilters are refused by name so the error says why.
//
// Each entry maps an option name to the kind of its value. pos lists the option names
// that positional arguments fill, in order (scale=540:960 is w=540:h=960).
type filterSpec struct {
	pos  []string
	opts map[string]kind
}

var (
	kExpr      = match(`^[A-Za-z0-9_+\-*/().,]{1,64}$`)
	kFlags     = match(`^[a-z_]{3,16}(\+[a-z_]{3,16}){0,3}$`)
	kPixList   = match(`^[a-z0-9]{3,12}(\|[a-z0-9]{3,12}){0,3}$`)
	kDB        = match(`^-[0-9]{1,3}dB$`)
	kBool      = enum("true", "false", "0", "1")
	kHexColour = match(`^#[0-9A-Fa-f]{6}$`)
	kSamples   = match(`^[0-9]{1,12}S$`)
)

// exprWords are the identifiers and functions an expression value may use.
var exprWords = map[string]bool{
	"min": true, "max": true, "iw": true, "ih": true, "ow": true, "oh": true,
	"PTS": true, "STARTPTS": true, "N": true, "TB": true,
}

var identRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

func kExprWords(s string) bool {
	if !kExpr(s) {
		return false
	}
	for _, w := range identRe.FindAllString(s, -1) {
		if !exprWords[w] {
			return false
		}
	}
	return true
}

var filters = map[string]filterSpec{
	"scale": {pos: []string{"w", "h"}, opts: map[string]kind{
		"w": kExprWords, "h": kExprWords, "flags": kFlags,
		"force_original_aspect_ratio": enum("decrease", "increase"),
		"in_range":                    enum("tv", "pc", "limited", "full"),
		"out_range":                   enum("tv", "pc", "limited", "full"),
		"in_color_matrix":             enum("bt601", "bt709"),
		"out_color_matrix":            enum("bt601", "bt709"),
	}},
	"fps": {
		pos:  []string{"fps"},
		opts: map[string]kind{"fps": kRational, "round": enum("near", "up", "down", "zero", "inf")},
	},
	"format":  {pos: []string{"pix_fmts"}, opts: map[string]kind{"pix_fmts": kPixList}},
	"setpts":  {pos: []string{"expr"}, opts: map[string]kind{"expr": kExprWords}},
	"asetpts": {pos: []string{"expr"}, opts: map[string]kind{"expr": kExprWords}},
	"split":   {pos: []string{"outputs"}, opts: map[string]kind{"outputs": intRange(2, 8)}},
	"asplit":  {pos: []string{"outputs"}, opts: map[string]kind{"outputs": intRange(2, 8)}},
	"setsar":  {pos: []string{"sar"}, opts: map[string]kind{"sar": enum("1")}},
	"crop": {
		pos:  []string{"w", "h", "x", "y"},
		opts: map[string]kind{"w": kExprWords, "h": kExprWords, "x": kExprWords, "y": kExprWords},
	},
	"pad": {
		pos:  []string{"w", "h", "x", "y", "color"},
		opts: map[string]kind{"w": kExprWords, "h": kExprWords, "x": kExprWords, "y": kExprWords, "color": kHexColour},
	},
	"bwdif": {
		opts: map[string]kind{
			"mode":   enum("send_frame", "send_field"),
			"parity": enum("auto", "tff", "bff"),
			"deint":  enum("interlaced", "all"),
		},
	},
	"yadif": {
		opts: map[string]kind{
			"mode":   enum("send_frame", "send_field"),
			"parity": enum("auto", "tff", "bff"),
			"deint":  enum("interlaced", "all"),
		},
	},
	"tonemap": {opts: map[string]kind{"tonemap": enum("hable", "clip", "mobius"), "desat": intRange(0, 10)}},
	"setparams": {opts: map[string]kind{
		"colorspace": enum("bt709", "bt2020nc", "smpte170m"), "color_primaries": enum("bt709", "bt2020", "smpte170m"),
		"color_trc": enum("bt709", "arib-std-b67", "smpte2084"), "range": enum("tv", "pc"),
	}},
	"zscale": {opts: map[string]kind{
		"min": enum(
			"709",
			"170m",
			"2020_ncl",
		),
		"pin": enum("709", "170m", "2020"),
		"tin": enum("709", "170m", "arib-std-b67", "smpte2084", "linear"),
		"rin": enum("tv", "pc", "limited", "full"),
		"m":   enum("709"),
		"p":   enum("709"),
		"t":   enum("709", "linear"),
		"r":   enum("limited", "full"),
		"d":   enum("ordered", "none"),
		"npl": intRange(1, 10000),
	}},
	"hwupload":  {opts: map[string]kind{}},
	"aresample": {pos: []string{"sample_rate"}, opts: map[string]kind{"sample_rate": intRange(8000, 192000)}},
	"aformat": {
		opts: map[string]kind{"sample_fmts": enum("s16", "fltp"), "channel_layouts": enum("stereo", "mono")},
	},
	"adelay":        {pos: []string{"delays"}, opts: map[string]kind{"delays": kSamples, "all": kBool}},
	"apad":          {opts: map[string]kind{"whole_len": intRange(1, 1<<30)}},
	"atrim":         {opts: map[string]kind{"start_sample": intRange(0, 1<<30), "end_sample": intRange(1, 1<<30)}},
	"silencedetect": {opts: map[string]kind{"noise": kDB, "d": match(`^[0-9]{1,3}(\.[0-9]{1,3})?$`)}},
	"ebur128":       {opts: map[string]kind{"peak": enum("true"), "framelog": enum("quiet")}},
	"atempo":        {pos: []string{"tempo"}, opts: map[string]kind{"tempo": match(`^[0-9]{1,2}(\.[0-9]{1,6})?$`)}},
}

// lavfiSources are the only -f lavfi inputs (FF-12) plus testsrc2 for the detection runs.
var lavfiSources = map[string]filterSpec{
	"color":    {opts: map[string]kind{"c": kHexColour, "s": kSize, "r": kRational, "d": kNumber}},
	"nullsrc":  {opts: map[string]kind{"s": kSize, "r": kRational, "d": kNumber}},
	"anullsrc": {opts: map[string]kind{"r": intRange(8000, 192000), "cl": enum("stereo", "mono"), "d": kNumber}},
	"testsrc2": {opts: map[string]kind{"s": kSize, "r": kRational, "d": kNumber}},
}

// forbiddenFilters are refused by name; anything not in filters is refused anyway.
var forbiddenFilters = map[string]bool{
	"movie": true, "amovie": true, "subtitles": true, "ass": true, "drawtext": true, "lut3d": true,
	"haldclut": true, "lut": true, "geq": true, "sendcmd": true, "zmq": true, "azmq": true,
	"frei0r": true, "ladspa": true, "lv2": true, "dnn_processing": true, "ocr": true,
	"sofalizer": true, "afir": true, "vidstabtransform": true, "deshake": true,
	"eq": true, "boxblur": true, "hqdn3d": true, "overlay": true,
}

const maxGraphLen = 4096

// splitTop splits s on sep outside single quotes and parentheses.
func splitTop(s string, sep byte) ([]string, bool) {
	var out []string
	depth, quoted, start := 0, false, 0
	for i := range len(s) {
		switch c := s[i]; {
		case c == '\'':
			quoted = !quoted
		case quoted:
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth < 0 {
				return nil, false
			}
		case c == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if quoted || depth != 0 {
		return nil, false
	}
	return append(out, s[start:]), true
}

var graphChars = regexp.MustCompile(`^[A-Za-z0-9_.:,;=\[\]()'+\-*/|#%]+$`)

// checkGraph validates a -vf, -af or -filter_complex value (FF-10, FF-11).
func checkGraph(argv []string, i int, g string) error {
	if len(g) > maxGraphLen || !graphChars.MatchString(g) {
		return violate("FF-10", argv, i, "a filter graph holds only letters, digits and the characters of the grammar")
	}
	chains, ok := splitTop(g, ';')
	if !ok {
		return violate("FF-10", argv, i, "unbalanced quote or parenthesis in a filter graph")
	}
	for _, chain := range chains {
		chain = strings.TrimSpace(chain)
		// Strip leading labels, then trailing labels.
		for strings.HasPrefix(chain, "[") {
			end := strings.IndexByte(chain, ']')
			if end < 0 {
				return violate("FF-10", argv, i, "unclosed link label")
			}
			chain = chain[end+1:]
		}
		for strings.HasSuffix(chain, "]") {
			start := strings.LastIndexByte(chain, '[')
			if start < 0 {
				return violate("FF-10", argv, i, "unopened link label")
			}
			chain = chain[:start]
		}
		if chain == "" {
			return violate("FF-10", argv, i, "an empty filter chain")
		}
		parts, ok := splitTop(chain, ',')
		if !ok {
			return violate("FF-10", argv, i, "unbalanced quote or parenthesis in a filter chain")
		}
		for _, f := range parts {
			if err := checkFilter(argv, i, f, filters); err != nil {
				return err
			}
		}
	}
	return nil
}

var filterName = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func checkFilter(argv []string, i int, f string, table map[string]filterSpec) error {
	name, args, _ := strings.Cut(f, "=")
	if !filterName.MatchString(name) {
		return violate("FF-10", argv, i, "%q is not a filter name", name)
	}
	if forbiddenFilters[name] {
		return violate("FF-10", argv, i, "filter %q reads files, loads code or is not in the LGPL build", name)
	}
	spec, ok := table[name]
	if !ok {
		return violate("FF-10", argv, i, "filter %q is not in the allowlist", name)
	}
	if args == "" {
		return nil
	}
	segs, ok := splitTop(args, ':')
	if !ok {
		return violate("FF-10", argv, i, "unbalanced quote or parenthesis in the arguments of %s", name)
	}
	pos := 0
	for _, seg := range segs {
		key, val, hasKey := strings.Cut(seg, "=")
		// A positional value may itself contain '=' only inside quotes or never; a key is a bare word.
		if hasKey && !filterName.MatchString(key) {
			key, val, hasKey = "", seg, false
		}
		if !hasKey {
			if pos >= len(spec.pos) {
				return violate("FF-10", argv, i, "filter %s takes no positional argument %q", name, seg)
			}
			key, val = spec.pos[pos], seg
			pos++
		}
		k, ok := spec.opts[key]
		if !ok {
			return violate("FF-10", argv, i, "filter %s has no allowed option %q", name, key)
		}
		if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
			val = val[1 : len(val)-1]
		}
		if strings.ContainsAny(val, "'") || !k(val) {
			return violate("FF-10", argv, i, "value %q of %s.%s is outside the grammar", val, name, key)
		}
	}
	return nil
}

// checkLavfi validates the source of -f lavfi -i <source> (FF-12).
func checkLavfi(argv []string, i int, src string) error {
	if len(src) > 256 || !graphChars.MatchString(src) || strings.ContainsAny(src, ",;[]") {
		return violate(
			"FF-12",
			argv,
			i,
			"a lavfi source is one of color, nullsrc, anullsrc and testsrc2 with numeric parameters",
		)
	}
	return checkFilter(argv, i, src, lavfiSources)
}

// FilterNames lists the filters the checker accepts, sorted: the ones a build must have for the argv of
// the engine to run (SDD-007 FF-10).
func FilterNames() []string {
	names := make([]string, 0, len(filters))
	for n := range filters {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
