package ffargs

import (
	"regexp"
	"strings"
)

// ffprobe has no -nostdin on the pinned build ("Option not found"), so FF-01 for ffprobe
// is "-v error -hide_banner"; the runner gives the child no standard input anyway.

const (
	// ProbeSize is the -probesize of FF-09: 32 MiB.
	ProbeSize = 33554432
	// ProbeAnalyze is the -analyzeduration of FF-09: 20 s in microseconds.
	ProbeAnalyze = 20000000
)

var (
	kShowEntries = match(
		`^[a-z_]{3,12}=[a-z_0-9]{3,24}(,[a-z_0-9]{3,24}){0,7}(:[a-z_]{3,12}=[a-z_0-9]{3,24}(,[a-z_0-9]{3,24}){0,7})?$`,
	)
	kSelect      = match(`^(v|a)(:[0-9]{1,2})?$|^[0-9]{1,3}$`)
	kPrintFormat = enum("json", "csv=p=0")
)

var showEntryRe = regexp.MustCompile(`^(stream|format|packet|frame)=`)

// ffprobeOptions are the options of the query forms in SDD-011 sections 3.1, 4.9 and 8.4.
// Each query option takes a value except the -show_* flags.
var ffprobeOptions = map[string]optSpec{
	"-v":                  {value: enum("error")},
	"-hide_banner":        {flag: true},
	"-probesize":          {value: enum("33554432")},
	"-analyzeduration":    {value: enum("20000000")},
	"-protocol_whitelist": {value: enum("file")},
	"-format_whitelist":   {value: enum(InputFormats)},
	"-print_format":       {value: kPrintFormat},
	"-of":                 {value: kPrintFormat},
	"-show_format":        {flag: true},
	"-show_streams":       {flag: true},
	"-show_chapters":      {flag: true},
	"-show_frames":        {flag: true},
	"-count_packets":      {flag: true},
	"-select_streams":     {value: kSelect},
	"-show_entries":       {value: kShowEntries},
	"-read_intervals":     {value: kReadInt},
}

func checkFFprobe(argv []string, pol Policy) error {
	if len(argv) < 4 || argv[0] != "-v" || argv[1] != "error" || argv[2] != "-hide_banner" {
		return violate("FF-01", argv, 0, "an ffprobe run starts with -v error -hide_banner")
	}
	seen, err := scanProbe(argv, pol)
	if err != nil {
		return err
	}
	for _, need := range []string{"-probesize", "-analyzeduration", "-protocol_whitelist", "-format_whitelist"} {
		if _, ok := seen[need]; !ok {
			return violate("FF-09", argv, -1, "%s is required before -i", need)
		}
	}
	if _, ok := seen["-show_frames"]; ok && seen["-read_intervals"] != "%+#1" {
		return violate("FF-09", argv, -1, "-show_frames only with -read_intervals %%+#1 (one frame)")
	}
	if e, ok := seen["-show_entries"]; ok {
		return checkEntries(argv, e, seen)
	}
	return nil
}

// checkEntries applies FF-09 to -show_entries: packets only as timestamps of one stream.
func checkEntries(argv []string, e string, seen map[string]string) error {
	if !showEntryRe.MatchString(e) {
		return violate("FF-09", argv, -1, "-show_entries names stream, format, packet or frame fields")
	}
	if !strings.Contains(e, "packet=") {
		return nil
	}
	if _, ok := seen["-select_streams"]; !ok || seen["-of"] != "csv=p=0" ||
		(e != "packet=pts_time,dts_time" && e != "packet=pts_time,flags" && e != "packet=pts_time,duration_time") {
		return violate("FF-09", argv, -1, "packet entries are only the timestamps of one selected stream as csv")
	}
	return nil
}

// scanProbe reads every option and the one input of an ffprobe argv.
func scanProbe(argv []string, pol Policy) (map[string]string, error) {
	seen := map[string]string{}
	inputs := 0
	for i := 3; i < len(argv); i++ {
		tok := argv[i]
		if tok == "-i" {
			if i+1 >= len(argv) {
				return nil, violate("FF-03", argv, i, "-i needs an input")
			}
			i++
			inputs++
			if err := checkProbeInput(argv, i, pol); err != nil {
				return nil, err
			}
			continue
		}
		spec, ok := ffprobeOptions[tok]
		if !ok {
			if tok == "-show_packets" {
				return nil, violate("FF-09", argv, i, "-show_packets is never used on untrusted input")
			}
			return nil, violate("FF-07", argv, i, "option %s is not in the ffprobe grammar", tok)
		}
		val := ""
		if !spec.flag {
			if i+1 >= len(argv) {
				return nil, violate("FF-07", argv, i, "option %s needs a value", tok)
			}
			i++
			val = argv[i]
			if !spec.value(val) {
				return nil, violate(ruleOf(tok), argv, i, "value of %s is outside the grammar", tok)
			}
		}
		if _, dup := seen[tok]; dup {
			return nil, violate("FF-07", argv, i, "option %s is given twice", tok)
		}
		seen[tok] = val
	}
	if inputs != 1 {
		return nil, violate("FF-03", argv, -1, "an ffprobe run has exactly one input")
	}
	return seen, nil
}

func checkProbeInput(argv []string, i int, pol Policy) error {
	p, ok := splitFilePrefix(argv[i])
	if !ok {
		return violate("FF-03", argv, i, "-i takes file:<absolute path>")
	}
	if err := checkFilePath("FF-03", argv, i, p, pathInput, pol); err != nil {
		return err
	}
	if i != len(argv)-1 {
		return violate("FF-03", argv, i, "-i is the last option of an ffprobe run")
	}
	return nil
}
