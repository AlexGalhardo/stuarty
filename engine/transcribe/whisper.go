package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem/contrib/engine"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

const (
	// ProviderWhisper is Name of the whisper.cpp provider.
	ProviderWhisper = "whisper.cpp"

	// maxOutputJSON caps the JSON the helper writes (SDD-011 section 10.2).
	maxOutputJSON = 16 << 20
	// maxJSONDepth bounds the nesting of that JSON; the real output is four levels deep.
	maxJSONDepth = 16

	wavName = "in.wav"
	outBase = "out"
	outName = outBase + ".json"
)

// ErrLanguage is a language that is not in the list of whisper.cpp.
var ErrLanguage = errors.New("transcribe: unsupported language")

// languages are the ISO 639-1 codes (and the few three-letter ones) whisper.cpp knows. A language
// outside this list never reaches the command line.
var languages = []string{
	"en", "zh", "de", "es", "ru", "ko", "fr", "ja", "pt", "tr", "pl", "ca", "nl", "ar", "sv", "it", "id",
	"hi", "fi", "vi", "he", "uk", "el", "ms", "cs", "ro", "da", "hu", "ta", "no", "th", "ur", "hr", "bg",
	"lt", "la", "mi", "ml", "cy", "sk", "te", "fa", "lv", "bn", "sr", "az", "sl", "kn", "et", "mk", "br",
	"eu", "is", "hy", "ne", "mn", "bs", "kk", "sq", "sw", "gl", "mr", "pa", "si", "km", "sn", "yo", "so",
	"af", "oc", "ka", "be", "tg", "sd", "gu", "am", "yi", "lo", "uz", "fo", "ht", "ps", "tk", "nn", "mt",
	"sa", "lb", "my", "bo", "tl", "mg", "as", "tt", "haw", "ln", "ha", "ba", "jw", "su", "yue",
}

// Language turns the requested language into the value of -l: "auto", or the primary subtag of a
// BCP 47 tag in lower case when whisper.cpp knows it.
func Language(tag string) (string, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" || tag == "auto" {
		return "auto", nil
	}
	primary, _, _ := strings.Cut(strings.ReplaceAll(tag, "_", "-"), "-")
	if !slices.Contains(languages, primary) {
		return "", fmt.Errorf("%w: %q", ErrLanguage, truncate(tag, 20))
	}
	return primary, nil
}

// Threads is the smaller of 8 and the physical cores, which are the logical CPUs halved on amd64 and
// 386 when there are at least 4 (SMT assumed) (ADR-0021).
func Threads(logical int) int {
	physical := logical
	if (runtime.GOARCH == "amd64" || runtime.GOARCH == "386") && logical >= 4 {
		physical = logical / 2
	}
	return min(8, max(1, physical))
}

// Argv is the command line of whisper-cli, from a fixed template: no flag carries user text. lang comes
// from Language; the paths are the model, the WAV and the prefix of the output, all in the run directory
// or the install folder.
func Argv(model, wav, lang string, threads int, outPrefix string) []string {
	return []string{
		"-m", model, "-f", wav, "-l", lang, "-t", strconv.Itoa(threads),
		"-ml", "1", "-sow", "-ojf", "-pp", "-of", outPrefix,
	}
}

// Whisper is the whisper.cpp provider: a separate process, installed by an explicit step, never
// downloaded here (ADR-0021 item 2).
type Whisper struct {
	// FFmpeg writes the 16 kHz mono WAV the helper reads.
	FFmpeg *ffmpeg.Runner
	// Helper is the path of whisper-cli; HelperVersion goes into the artifact.
	Helper        string
	HelperVersion string
	// ModelName is "small", "base" ...; ModelPath the file under tools/models; ModelSHA256 its pinned hash.
	ModelName   string
	ModelPath   string
	ModelSHA256 string
	// CPUCount is the CPU budget (engine.Tool.CPUCount); the machine's when zero.
	CPUCount int
	// Env widens the environment of the helper, for example LD_LIBRARY_PATH on Linux.
	Env map[string]string
	// Limits bound the helper (Linux and Windows; macOS has no mechanism).
	Limits engine.Limits
	// HelperItem and ModelItem are what Check reports as missing: the manifest's name, host and size.
	HelperItem, ModelItem Item

	// tool is the resolved helper; tests set it to run a fake one.
	tool *engine.Tool
}

// Name implements Provider.
func (*Whisper) Name() string { return ProviderWhisper }

// Model implements Provider.
func (w *Whisper) Model() string { return w.ModelName }

// Check says whether the helper and the model are in place. It never downloads.
func (w *Whisper) Check(context.Context) error {
	var m MissingError
	if !isFile(w.Helper) {
		m.Items = append(m.Items, w.HelperItem)
		m.DownloadBytes += w.HelperItem.Bytes
	}
	if !isFile(w.ModelPath) {
		m.Items = append(m.Items, w.ModelItem)
		m.DownloadBytes += w.ModelItem.Bytes
	}
	if len(m.Items) > 0 {
		return &m
	}
	return nil
}

func isFile(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func (w *Whisper) helper(ctx context.Context) (*engine.Tool, error) {
	if w.tool != nil {
		return w.tool, nil
	}
	t, err := engine.Find(ctx, engine.Config{
		Name:        "whisper-cli",
		Path:        w.Helper,
		VersionArgs: []string{},
		Env:         w.Env,
		Limits:      w.Limits,
		CPUCount:    w.CPUCount,
	})
	if err != nil {
		return nil, &MissingError{Items: []Item{w.HelperItem}, DownloadBytes: w.HelperItem.Bytes}
	}
	w.tool = t
	return t, nil
}

// Transcribe writes the 16 kHz mono WAV of the asset's audio into the run directory, runs whisper-cli on
// it and reads its JSON. progress is called with the audio time the helper has decoded.
func (w *Whisper) Transcribe(
	ctx context.Context,
	in Input,
	progress func(done, total time.Duration),
) (Raw, error) {
	lang, err := Language(in.Language)
	if err != nil {
		return Raw{}, err
	}
	// First: a helper that is not installed is reported before anything else, and nothing is downloaded.
	if err := w.Check(ctx); err != nil {
		return Raw{}, err
	}
	if !filepath.IsAbs(w.Helper) || !filepath.IsAbs(w.ModelPath) {
		return Raw{}, failure(
			ReasonPrepare,
			"The transcription helper is not configured with absolute paths.",
			errNotAbsolute,
			"",
		)
	}
	if in.WorkDir == nil || w.FFmpeg == nil {
		return Raw{}, failure(
			ReasonPrepare,
			"The transcription job is incomplete.",
			errors.New("run directory and FFmpeg are required"),
			"",
		)
	}
	tool, err := w.helper(ctx)
	if err != nil {
		return Raw{}, err
	}

	wav, err := in.WorkDir.Path(wavName)
	if err != nil {
		return Raw{}, failure(ReasonPrepare, "The work folder is not usable.", err, "")
	}
	if err := w.prepareAudio(ctx, in, wav); err != nil {
		return Raw{}, err
	}

	prefix, err := in.WorkDir.Path(outBase)
	if err != nil {
		return Raw{}, failure(ReasonPrepare, "The work folder is not usable.", err, "")
	}
	cpus := w.CPUCount
	if cpus <= 0 {
		cpus = runtime.NumCPU()
	}
	argv := Argv(w.ModelPath, wav, lang, Threads(cpus), prefix)

	raw, err := runHelper(ctx, tool, in.WorkDir.Dir(), argv, in.Duration, progress)
	if err != nil {
		return Raw{}, err
	}
	raw.Info = map[string]string{
		"provider": ProviderWhisper, "provider_version": w.HelperVersion,
		"model": w.ModelName, "model_sha256": w.ModelSHA256,
	}
	return raw, nil
}

// prepareAudio is the FFmpeg run of SDD-011 section 10.2: mono 16 kHz signed 16-bit WAV. Whisper never
// sees the original file, and never a corrupt one.
func (w *Whisper) prepareAudio(ctx context.Context, in Input, wav string) error {
	return extractWAV(ctx, w.FFmpeg, in, wav, 0, 0)
}

// extractWAV writes the audio of in, from startMS for lengthMS (the whole audio when lengthMS is zero), as
// mono 16 kHz signed 16-bit WAV at the path wav in the run directory.
func extractWAV(ctx context.Context, r *ffmpeg.Runner, in Input, wav string, startMS, lengthMS int64) error {
	opts := []ffargs.Opt{ffargs.Int("-ac", 1), ffargs.Int("-ar", 16000), ffargs.Enum("-c:a", "pcm_s16le")}
	if lengthMS > 0 {
		opts = append(opts, ffargs.Enum("-t", strconv.FormatFloat(float64(lengthMS)/1000, 'f', 3, 64)))
	}

	argv, err := ffargs.Cmd{
		LogErrorOnly: true,
		Inputs:       []ffargs.Input{ffargs.FileInput(in.Audio).At(startMS)},
		Outputs: []ffargs.Output{{
			Format: "wav", Path: wav, Maps: []string{"0:a:0"}, NoVideo: true, NoSubs: true, NoData: true, Opts: opts,
		}},
	}.Argv(r.Policy(in.WorkDir, in.InputDirs...))
	if err != nil {
		return failure(ReasonPrepare, "The audio could not be prepared for transcription.", err, "")
	}

	res, err := r.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: in.WorkDir, Argv: argv, InputDirs: in.InputDirs, Deadline: ffmpeg.AudioExtractDeadline(in.Duration),
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return failure(ReasonPrepare, "The audio could not be prepared for transcription.", err, res.Stderr)
	}

	return nil
}

// runHelper runs the helper in dir and judges the run by its output file: exit code 0 with no JSON is a
// failure, because whisper-cli exits 0 on corrupt audio (ADR-0021).
func runHelper(
	ctx context.Context,
	tool *engine.Tool,
	dir string,
	argv []string,
	audio time.Duration,
	progress func(done, total time.Duration),
) (Raw, error) {
	rctx, cancel := context.WithTimeout(ctx, ffmpegTranscribeDeadline(audio))
	defer cancel()

	pw := &progressWriter{total: audio, on: progress}
	res, err := tool.RunStream(rctx, dir, pw, argv...)
	if ctx.Err() != nil {
		return Raw{}, ctx.Err()
	}
	if err != nil {
		if errors.Is(rctx.Err(), context.DeadlineExceeded) {
			return Raw{}, failure(ReasonTimeout, "The transcription took too long and was ended.", err, res.Stderr)
		}
		return Raw{}, failure(ReasonExitCode, "The transcription program failed.", err, res.Stderr)
	}

	path, err := engine.Sanitized(dir, outName)
	if err != nil {
		return Raw{}, failure(ReasonOutputMissing, "The transcription program wrote no result.", err, res.Stderr)
	}
	data, err := readCapped(path)
	switch {
	case errors.Is(err, errTooBig):
		return Raw{}, failure(ReasonOutputTooBig, "The transcription result is too large.", err, "")
	case err != nil || len(bytes.TrimSpace(data)) == 0:
		return Raw{}, failure(ReasonOutputMissing, "The transcription program wrote no result.", err, res.Stderr)
	}
	raw, err := ParseWhisperJSON(data, audio.Milliseconds())
	if err != nil {
		return Raw{}, failure(ReasonOutputMissing, "The transcription result could not be read.", err, res.Stderr)
	}
	return raw, nil
}

func ffmpegTranscribeDeadline(audio time.Duration) time.Duration {
	return ffmpeg.TranscribeDeadline(audio)
}

// errNotAbsolute: the engine resolves a relative program through the PATH or the working directory, which
// SDD-007 section 6.8 forbids for a helper.
var errNotAbsolute = errors.New("the helper and the model must be absolute paths")

var errTooBig = errors.New("the output is over the cap")

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxOutputJSON+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxOutputJSON {
		return nil, errTooBig
	}
	return data, nil
}

// progressWriter reads the transcript lines whisper-cli prints on standard output,
// "[00:00:01.000 --> 00:00:04.320] text", and reports the end time of each.
type progressWriter struct {
	total time.Duration
	on    func(done, total time.Duration)
	buf   []byte
}

var stampRe = regexp.MustCompile(`^\[(\d+):(\d{2}):(\d{2})[.,](\d{3}) --> (\d+):(\d{2}):(\d{2})[.,](\d{3})\]`)

func (p *progressWriter) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		p.line(string(p.buf[:i]))
		p.buf = p.buf[i+1:]
	}
	if len(p.buf) > 1<<16 {
		p.buf = p.buf[:0]
	}
	return len(b), nil
}

func (p *progressWriter) line(s string) {
	m := stampRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil || p.on == nil {
		return
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	end := time.Duration(n(5))*time.Hour + time.Duration(n(6))*time.Minute +
		time.Duration(n(7))*time.Second + time.Duration(n(8))*time.Millisecond
	p.on(min(end, p.total), p.total)
}

// failure is the E_TRANSCRIBE_FAILED of a run. The stderr tail is untrusted text: only the first line
// that says "error" is kept, with every path removed.
func failure(reason, msg string, cause error, stderr string) *ffmpeg.Error {
	details := map[string]any{"reason": reason}
	if line := errorLine(stderr); line != "" {
		details["stderr"] = line
	}
	return &ffmpeg.Error{Code: CodeFailed, Reason: reason, Message: msg, Details: details, Err: cause}
}

func errorLine(stderr string) string {
	for line := range strings.SplitSeq(stderr, "\n") {
		if strings.Contains(strings.ToLower(line), "error") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if strings.ContainsAny(f, `/\`) {
					fields[i] = "<path>"
				}
			}
			return truncate(cleanText(strings.Join(fields, " ")), 200)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// cleanText replaces invalid UTF-8 with U+FFFD and removes control characters and the line, paragraph
// and bidirectional controls (SDD-011 section 10.7).
func cleanText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), r == 0x2028, r == 0x2029, r == 0x200E, r == 0x200F,
			r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
			return -1
		}
		return r
	}, s)
}

// whisper JSON, as written by -ojf (spike H4).
type whisperJSON struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription *[]whisperEntry `json:"transcription"`
}

type whisperEntry struct {
	Offsets struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"offsets"`
	Text   string         `json:"text"`
	Tokens []whisperToken `json:"tokens"`
}

type whisperToken struct {
	Text string  `json:"text"`
	P    float64 `json:"p"`
}

// specialRe matches the special tokens: [_BEG_], [_SOT_] and the timestamp tokens [_TT_308]. The pattern
// of SDD-011 section 10.2, ^\[_.*_\]$, misses the last kind, which ends in digits.
var specialRe = regexp.MustCompile(`^\[_[A-Za-z]+(_\d+)?_?\]$`)

// ParseWhisperJSON reads the output of whisper-cli -ml 1 -sow -ojf (SDD-011 section 10.2, Parsing). Every
// string in it is untrusted text.
func ParseWhisperJSON(data []byte, audioMS int64) (Raw, error) {
	if len(data) > maxOutputJSON {
		return Raw{}, errTooBig
	}
	if depth(data) > maxJSONDepth {
		return Raw{}, errors.New("the output is nested too deeply")
	}
	var doc whisperJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return Raw{}, err
	}
	if doc.Transcription == nil {
		return Raw{}, errors.New("the output has no transcription")
	}

	var words []RawWord
	for i, e := range *doc.Transcription {
		text := cleanText(e.Text)
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || specialRe.MatchString(trimmed) {
			continue
		}
		var probs []float64
		for _, t := range e.Tokens {
			tt := strings.TrimSpace(t.Text)
			if specialRe.MatchString(tt) || !hasLetterOrDigit(tt) {
				continue
			}
			probs = append(probs, t.P)
		}
		continues := !strings.HasPrefix(text, " ") && i > 0 && len(words) > 0
		if continues || (len(words) > 0 && punctuationOnly(trimmed)) {
			last := &words[len(words)-1]
			last.Text += trimmed
			last.EndMS = max(last.EndMS, e.Offsets.To)
			last.TokenProbs = append(last.TokenProbs, probs...)
			continue
		}
		words = append(words, RawWord{
			Text: trimmed, StartMS: e.Offsets.From, EndMS: e.Offsets.To, TokenProbs: probs, Confidence: math.NaN(),
		})
	}

	sort.SliceStable(words, func(a, b int) bool { return words[a].StartMS < words[b].StartMS })
	var prevEnd int64
	for i := range words {
		w := &words[i]
		if w.StartMS < prevEnd {
			w.StartMS = prevEnd
		}
		w.StartMS = min(max(w.StartMS, 0), audioMS)
		w.EndMS = min(max(w.EndMS, w.StartMS), audioMS)
		prevEnd = w.EndMS
	}
	return Raw{Language: cleanText(doc.Result.Language), Words: words}, nil
}

func hasLetterOrDigit(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

func punctuationOnly(s string) bool {
	return !hasLetterOrDigit(s)
}

// depth is the deepest nesting of arrays and objects in the JSON text, string contents excluded.
func depth(data []byte) int {
	var cur, most int
	inString, escaped := false, false
	for _, c := range data {
		switch {
		case escaped:
			escaped = false
		case inString:
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{' || c == '[':
			cur++
			most = max(most, cur)
		case c == '}' || c == ']':
			cur--
		}
	}
	return most
}
