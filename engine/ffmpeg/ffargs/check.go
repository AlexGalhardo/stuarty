package ffargs

import (
	"slices"
	"strconv"
	"strings"
)

type option struct {
	idx   int
	name  string
	value string
}

// scanner is the state of one pass over an ffmpeg argv.
type scanner struct {
	argv              []string
	pol               Policy
	pending           []option
	inputs, outputs   int
	seenN, seenAlloc  bool
	progress, pipeOut bool
}

// checkFFmpeg implements FF-01 to FF-16 as a scanner over an ffmpeg argv.
func checkFFmpeg(argv []string, pol Policy) error {
	if len(argv) == 2 && argv[0] == "-hide_banner" && slices.Contains(infoFlags, argv[1]) {
		return nil
	}
	if len(argv) < 3 || argv[0] != "-nostdin" || argv[1] != "-hide_banner" || argv[2] != "-nostats" {
		return violate("FF-01", argv, 0, "an ffmpeg run starts with -nostdin -hide_banner -nostats")
	}
	s := &scanner{argv: argv, pol: pol}
	for i := 3; i < len(argv); i++ {
		next, err := s.step(i)
		if err != nil {
			return err
		}
		i = next
	}
	return s.finish()
}

// step handles the token at i and returns the index of the last token it consumed.
func (s *scanner) step(i int) (int, error) {
	tok := s.argv[i]
	switch {
	case tok == "-i":
		return s.input(i)
	case strings.HasPrefix(tok, "-") && tok != "-":
		return s.option(i)
	}
	return i, s.output(i)
}

func (s *scanner) input(i int) (int, error) {
	if i+1 >= len(s.argv) {
		return i, violate("FF-03", s.argv, i, "-i needs an input")
	}
	i++
	if err := checkInput(s.argv, i, s.pending, s.argv[i], s.pol); err != nil {
		return i, err
	}
	s.inputs++
	if s.inputs > 8 {
		return i, violate("FF-03", s.argv, i, "at most 8 inputs")
	}
	s.pending = nil
	return i, nil
}

// readOption reads an option and its value and checks the value against the grammar.
func (s *scanner) readOption(i int) (optSpec, option, int, error) {
	tok := s.argv[i]
	if tok == "-y" {
		return optSpec{}, option{}, i, violate("FF-14", s.argv, i, "-y is never used; outputs are written once with -n")
	}
	spec, ok := ffmpegOptions[tok]
	if !ok {
		return spec, option{}, i, violate("FF-07", s.argv, i, "option %s is not in the grammar", tok)
	}
	o := option{idx: i, name: tok}
	if spec.flag {
		return spec, o, i, nil
	}
	if i+1 >= len(s.argv) {
		return spec, o, i, violate("FF-07", s.argv, i, "option %s needs a value", tok)
	}
	i++
	o.value = s.argv[i]
	if strings.HasPrefix(o.value, "@") {
		return spec, o, i, violate("FF-10", s.argv, i, "@file option values are refused")
	}
	switch tok {
	case "-filter_complex", "-vf", "-af":
		return spec, o, i, checkGraph(s.argv, i, o.value)
	}
	if !spec.value(o.value) {
		return spec, o, i, violate(ruleOf(tok), s.argv, i, "value of %s is outside the grammar", tok)
	}
	return spec, o, i, nil
}

func (s *scanner) option(i int) (int, error) {
	spec, o, i, err := s.readOption(i)
	if err != nil {
		return i, err
	}
	if spec.class == classGlobal {
		switch o.name {
		case "-n":
			s.seenN = true
		case "-max_alloc":
			s.seenAlloc = true
		case "-progress":
			s.progress = true
		case "-filter_threads", "-filter_complex_threads":
			return i, checkThreads(s.argv, i, o.value, s.pol)
		}
		return i, nil
	}
	if o.name == "-threads" {
		if err := checkThreads(s.argv, i, o.value, s.pol); err != nil {
			return i, err
		}
	}
	s.pending = append(s.pending, o)
	return i, nil
}

// output handles a path (or "-" for the null muxer) and the options pending before it.
func (s *scanner) output(i int) error {
	if s.inputs == 0 {
		return violate("FF-03", s.argv, i, "an output before any input")
	}
	if !s.seenN || !s.seenAlloc {
		return violate("FF-01", s.argv, i, "-n and -max_alloc %d come before the first input", MaxAlloc)
	}
	if err := checkOutput(s.argv, i, s.pending, s.argv[i], s.pol, &s.pipeOut); err != nil {
		return err
	}
	s.outputs++
	s.pending = nil
	return nil
}

func (s *scanner) finish() error {
	switch {
	case len(s.pending) != 0:
		return violate("FF-14", s.argv, s.pending[0].idx, "option %s is not followed by an output", s.pending[0].name)
	case s.inputs == 0 || s.outputs == 0:
		return violate("FF-03", s.argv, -1, "an ffmpeg run needs an input and an output")
	case !s.seenN || !s.seenAlloc:
		return violate("FF-01", s.argv, -1, "-n and -max_alloc %d are required", MaxAlloc)
	case s.progress && s.pipeOut:
		return violate("FF-13", s.argv, -1, "-progress pipe:1 and an output on pipe:1 cannot share standard output")
	}
	return nil
}

func ruleOf(opt string) string {
	switch opt {
	case "-protocol_whitelist", "-format_whitelist", "-sn", "-dn":
		return "FF-02"
	case "-f":
		return "FF-13"
	case "-threads", "-filter_threads", "-filter_complex_threads":
		return "FF-16"
	case "-map", "-map_metadata", "-map_chapters":
		return "FF-08"
	case "-c:v", "-c:a", "-q:v", "-b:v", "-b:a", "-crf", "-g", "-bf", "-profile:v", "-preset", "-quality", "-lossless":
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

func has(opts []option, name string) bool {
	return slices.ContainsFunc(opts, func(o option) bool { return o.name == name })
}

// inputDecoder is the one decoder an input may force: it carries the alpha plane of VP9 in WebM.
const inputDecoder = "libvpx-vp9"

var inputOptions = []string{
	"-protocol_whitelist", "-format_whitelist", "-sn", "-dn", "-ss", "-t", "-threads",
	"-pix_fmt", "-s", "-framerate", "-ar", "-ac", "-pattern_type", "-c:v",
}

// inputFormat checks the options before an -i and returns the format they force, if any.
func inputFormat(argv []string, i int, opts []option) (string, error) {
	// Order of FF-02: -protocol_whitelist first.
	if len(opts) == 0 || opts[0].name != "-protocol_whitelist" {
		return "", violate("FF-02", argv, i, "-protocol_whitelist file,pipe must come first before every -i")
	}
	forced := ""
	for _, o := range opts {
		if o.name == "-f" {
			if o.value != "lavfi" && o.value != "rawvideo" && o.value != "s16le" {
				return "", violate("FF-13", argv, o.idx, "an input format is lavfi, rawvideo or s16le")
			}
			forced = o.value
			continue
		}
		if o.name == "-c:v" && o.value != inputDecoder {
			return "", violate("FF-15", argv, o.idx, "the only input decoder is %s", inputDecoder)
		}
		if !slices.Contains(inputOptions, o.name) {
			return "", violate("FF-07", argv, o.idx, "option %s is not an input option", o.name)
		}
	}
	if forced != "" && has(opts, "-c:v") {
		return "", violate("FF-15", argv, i, "a forced decoder is for a file input")
	}
	return forced, nil
}

// checkPipeInput requires the explicit format and sizes of a pipe:0 input (FF-03).
func checkPipeInput(argv []string, i int, opts []option, forced string) error {
	var need []string
	switch forced {
	case "rawvideo":
		need = []string{"-pix_fmt", "-s", "-framerate"}
	case "s16le":
		need = []string{"-ar", "-ac"}
	default:
		return violate("FF-03", argv, i, "a pipe input carries an explicit -f rawvideo or -f s16le")
	}
	for _, n := range need {
		if !has(opts, n) {
			return violate("FF-03", argv, i, "a %s pipe input carries %s", forced, n)
		}
	}
	return nil
}

func checkInput(argv []string, i int, opts []option, path string, pol Policy) error {
	forced, err := inputFormat(argv, i, opts)
	if err != nil {
		return err
	}
	switch {
	case forced == "lavfi":
		if path == "" || strings.Contains(path, ":/") {
			return violate("FF-12", argv, i, "a lavfi input is a source, not a path")
		}
		return checkLavfi(argv, i, path)
	case path == "pipe:0":
		return checkPipeInput(argv, i, opts, forced)
	}

	p, ok := splitFilePrefix(path)
	if !ok {
		return violate("FF-03", argv, i, "-i takes file:<absolute path> or pipe:0")
	}
	if forced != "" {
		return violate("FF-13", argv, i, "a file input is probed, never forced with -f")
	}
	if len(opts) < 4 || opts[1].name != "-format_whitelist" || opts[2].name != "-sn" || opts[3].name != "-dn" {
		return violate(
			"FF-02",
			argv,
			i,
			"-format_whitelist, -sn and -dn follow -protocol_whitelist before a file input",
		)
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
	if !slices.Contains(outputFormats, format) {
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
