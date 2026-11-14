package ffargs

import (
	"strconv"
	"strings"
)

type option struct {
	idx   int
	name  string
	value string
}

// checkFFmpeg implements FF-01 to FF-16 as a scanner over an ffmpeg argv.
func checkFFmpeg(argv []string, pol Policy) error {
	if len(argv) == 2 && argv[0] == "-hide_banner" {
		for _, f := range infoFlags {
			if argv[1] == f {
				return nil
			}
		}
	}
	if len(argv) < 3 || argv[0] != "-nostdin" || argv[1] != "-hide_banner" || argv[2] != "-nostats" {
		return violate("FF-01", argv, 0, "an ffmpeg run starts with -nostdin -hide_banner -nostats")
	}

	var (
		pending   []option
		inputs    int
		outputs   int
		seenN     bool
		seenAlloc bool
		progress  bool
		pipeOut   bool
	)

	for i := 3; i < len(argv); i++ {
		tok := argv[i]

		if tok == "-i" {
			if i+1 >= len(argv) {
				return violate("FF-03", argv, i, "-i needs an input")
			}
			i++
			if err := checkInput(argv, i, pending, argv[i], pol); err != nil {
				return err
			}
			inputs++
			if inputs > 8 {
				return violate("FF-03", argv, i, "at most 8 inputs")
			}
			pending = nil
			continue
		}

		if strings.HasPrefix(tok, "-") && tok != "-" {
			if tok == "-y" {
				return violate("FF-14", argv, i, "-y is never used; outputs are written once with -n")
			}
			spec, ok := ffmpegOptions[tok]
			if !ok {
				return violate("FF-07", argv, i, "option %s is not in the grammar", tok)
			}
			o := option{idx: i, name: tok}
			if !spec.flag {
				if i+1 >= len(argv) {
					return violate("FF-07", argv, i, "option %s needs a value", tok)
				}
				i++
				o.value = argv[i]
				if strings.HasPrefix(o.value, "@") {
					return violate("FF-10", argv, i, "@file option values are refused")
				}
				switch tok {
				case "-filter_complex", "-vf", "-af":
					if err := checkGraph(argv, i, o.value); err != nil {
						return err
					}
				default:
					if !spec.value(o.value) {
						return violate(ruleOf(tok), argv, i, "value of %s is outside the grammar", tok)
					}
				}
			}
			switch spec.class {
			case classGlobal:
				switch tok {
				case "-n":
					seenN = true
				case "-max_alloc":
					seenAlloc = true
				case "-progress":
					progress = true
				case "-filter_threads":
					if err := checkThreads(argv, i, o.value, pol); err != nil {
						return err
					}
				}
			default:
				if tok == "-threads" {
					if err := checkThreads(argv, i, o.value, pol); err != nil {
						return err
					}
				}
				pending = append(pending, o)
			}
			continue
		}

		// An output: a path or "-" for the null muxer.
		if inputs == 0 {
			return violate("FF-03", argv, i, "an output before any input")
		}
		if !seenN || !seenAlloc {
			return violate("FF-01", argv, i, "-n and -max_alloc %d come before the first input", MaxAlloc)
		}
		if err := checkOutput(argv, i, pending, tok, pol, &pipeOut); err != nil {
			return err
		}
		outputs++
		pending = nil
	}

	if len(pending) != 0 {
		return violate("FF-14", argv, pending[0].idx, "option %s is not followed by an output", pending[0].name)
	}
	if inputs == 0 || outputs == 0 {
		return violate("FF-03", argv, -1, "an ffmpeg run needs an input and an output")
	}
	if !seenN || !seenAlloc {
		return violate("FF-01", argv, -1, "-n and -max_alloc %d are required", MaxAlloc)
	}
	if progress && pipeOut {
		return violate("FF-13", argv, -1, "-progress pipe:1 and an output on pipe:1 cannot share standard output")
	}
	return nil
}

func ruleOf(opt string) string {
	switch opt {
	case "-protocol_whitelist", "-format_whitelist", "-sn", "-dn":
		return "FF-02"
	case "-f":
		return "FF-13"
	case "-threads", "-filter_threads":
		return "FF-16"
	case "-map", "-map_metadata", "-map_chapters":
		return "FF-08"
	case "-c:v", "-c:a", "-q:v", "-b:v", "-b:a", "-crf", "-g", "-bf", "-profile:v", "-preset":
		return "FF-15"
	}
	return "FF-07"
}

func checkThreads(argv []string, i int, v string, pol Policy) error {
	n, _ := strconv.Atoi(v)
	if pol.MaxThreads <= 0 || n > pol.MaxThreads {
		return violate("FF-16", argv, i, "thread count %s is above the CPU budget (%d)", v, pol.MaxThreads)
	}
	return nil
}

func has(opts []option, name string) (option, bool) {
	for _, o := range opts {
		if o.name == name {
			return o, true
		}
	}
	return option{}, false
}

func checkInput(argv []string, i int, opts []option, path string, pol Policy) error {
	// Order of FF-02: -protocol_whitelist, then -format_whitelist and -sn -dn for a file.
	if len(opts) == 0 || opts[0].name != "-protocol_whitelist" {
		return violate("FF-02", argv, i, "-protocol_whitelist file,pipe must come first before every -i")
	}
	forced := ""
	for _, o := range opts {
		switch o.name {
		case "-f":
			if o.value != "lavfi" && o.value != "rawvideo" && o.value != "s16le" {
				return violate("FF-13", argv, o.idx, "an input format is lavfi, rawvideo or s16le")
			}
			forced = o.value
		case "-protocol_whitelist", "-format_whitelist", "-sn", "-dn", "-ss", "-t", "-threads", "-pix_fmt", "-s", "-framerate",
			"-ar", "-ac", "-pattern_type":
		default:
			return violate("FF-07", argv, o.idx, "option %s is not an input option", o.name)
		}
	}

	switch {
	case forced == "lavfi":
		if path == "" || strings.Contains(path, ":/") {
			return violate("FF-12", argv, i, "a lavfi input is a source, not a path")
		}
		return checkLavfi(argv, i, path)
	case path == "pipe:0":
		switch forced {
		case "rawvideo":
			for _, need := range []string{"-pix_fmt", "-s", "-framerate"} {
				if _, ok := has(opts, need); !ok {
					return violate("FF-03", argv, i, "a rawvideo pipe input carries %s", need)
				}
			}
		case "s16le":
			for _, need := range []string{"-ar", "-ac"} {
				if _, ok := has(opts, need); !ok {
					return violate("FF-03", argv, i, "an s16le pipe input carries %s", need)
				}
			}
		default:
			return violate("FF-03", argv, i, "a pipe input carries an explicit -f rawvideo or -f s16le")
		}
		return nil
	}

	p, ok := splitFilePrefix(path)
	if !ok {
		return violate("FF-03", argv, i, "-i takes file:<absolute path> or pipe:0")
	}
	if forced != "" {
		return violate("FF-13", argv, i, "a file input is probed, never forced with -f")
	}
	if len(opts) < 4 || opts[1].name != "-format_whitelist" || opts[2].name != "-sn" || opts[3].name != "-dn" {
		return violate("FF-02", argv, i, "-format_whitelist, -sn and -dn follow -protocol_whitelist before a file input")
	}
	return checkFilePath("FF-03", argv, i, p, pathInput, pol)
}

func checkOutput(argv []string, i int, opts []option, path string, pol Policy, pipeOut *bool) error {
	format := ""
	var maps, meta, chap int
	for _, o := range opts {
		switch o.name {
		case "-f":
			format = o.value
		case "-map":
			maps++
		case "-map_metadata":
			meta++
		case "-map_chapters":
			chap++
		case "-protocol_whitelist", "-format_whitelist", "-pattern_type", "-framerate", "-s":
			return violate("FF-07", argv, o.idx, "option %s is an input option", o.name)
		case "-sn", "-dn", "-ss", "-t":
			// -sn and -dn drop streams of an output; -ss and -t trim it.
		}
	}
	if format == "" {
		return violate("FF-13", argv, i, "every output names its format with -f")
	}
	ok := false
	for _, f := range outputFormats {
		ok = ok || f == format
	}
	if !ok || format == "lavfi" {
		return violate("FF-13", argv, i, "output format %q is not allowed", format)
	}
	if maps == 0 {
		return violate("FF-08", argv, i, "-map is explicit on every output")
	}
	if meta != 1 || chap != 1 {
		return violate("FF-08", argv, i, "-map_metadata -1 and -map_chapters -1 are required on every output")
	}

	switch format {
	case "null":
		if path != "-" {
			return violate("FF-13", argv, i, "the null muxer writes to -")
		}
		return nil
	case "rawvideo", "s16le":
		if path != "pipe:1" {
			return violate("FF-13", argv, i, "format %s writes to pipe:1 only", format)
		}
		*pipeOut = true
		return nil
	}
	p, isFile := splitFilePrefix(path)
	if !isFile {
		return violate("FF-14", argv, i, "an output is file:<run directory>/<name> or pipe:1")
	}
	return checkFilePath("FF-14", argv, i, p, pathOutput, pol)
}
