package transcribe

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

// Variables of the OpenAI-compatible provider (SDD-000 section 10). Sending audio out is an operator
// decision: with the URL and the key unset the provider does not exist.
const (
	EnvAPIURL   = "STUARTY_TRANSCRIBE_URL"
	EnvAPIKey   = "STUARTY_TRANSCRIBE_KEY"
	EnvAPIModel = "STUARTY_TRANSCRIBE_MODEL"

	// ProviderAPI is Name of the API provider.
	ProviderAPI = "openai-compatible"
	// DefaultAPIModel is the model when STUARTY_TRANSCRIBE_MODEL is unset.
	DefaultAPIModel = "whisper-1"

	// apiFileName is the only file name that leaves the machine: never the name or the path of an asset.
	apiFileName = "audio.wav"

	maxAPIResponse = 16 << 20
	// chunkMS is the longest part sent in one request: 10 minutes of 16 kHz mono WAV are 19 MB, under
	// the 25 MB most servers accept.
	chunkMS       = 600_000
	chunkSearchMS = 30_000
	maxChunkBytes = 24 << 20

	ReasonAPIUnauthorized = "api_unauthorized"
	ReasonAPIError        = "api_error"
	ReasonAPINoWords      = "api_no_word_timestamps"
	ReasonAPIRedirect     = "api_redirect_refused"
	ReasonAPITooLarge     = "api_response_too_large"
	ReasonAPIUnreachable  = "api_unreachable"
)

// ErrAPIConfig is an unsafe or incomplete configuration of the provider; the service turns it into the
// start-up error "unsafe transcription endpoint" (SDD-007 section 4.5).
var ErrAPIConfig = errors.New("transcribe: unsafe transcription endpoint")

// APIConfig is what the operator configured.
type APIConfig struct {
	URL, Key, Model string
	// FFmpeg cuts the parts that are sent.
	FFmpeg *ffmpeg.Runner
	// Backoff are the waits before each retry; 2 s, 8 s and 20 s when empty.
	Backoff []time.Duration
	// Timeout bounds one request; 10 minutes when zero.
	Timeout time.Duration
	// Test trusts a local server; production code leaves it nil.
	Test *outbound.TestOptions
}

// doer is the client of internal/outbound, seen only through what the provider needs of it.
type doer interface {
	Do(*http.Request) (*http.Response, error)
}

// API is the provider that calls an OpenAI-compatible transcription endpoint (ADR-0021 item 3).
type API struct {
	cfg      APIConfig
	endpoint *url.URL
	client   doer
}

// APIFromEnv makes the provider from the environment. With the URL and the key both unset it returns
// nil and no error: the provider is not constructed and nothing can dial. A key without a URL, a URL
// without a key, or an unsafe URL is an error.
func APIFromEnv(getenv func(string) string, r *ffmpeg.Runner) (*API, error) {
	rawURL, key := strings.TrimSpace(getenv(EnvAPIURL)), getenv(EnvAPIKey)
	if rawURL == "" && key == "" {
		return nil, nil //nolint:nilnil // not configured is not an error, and there is no provider
	}

	return NewAPI(APIConfig{URL: rawURL, Key: key, Model: getenv(EnvAPIModel), FFmpeg: r})
}

// NewAPI makes the provider. The URL must be https, or http for a loopback host, with no userinfo.
func NewAPI(cfg APIConfig) (*API, error) {
	if cfg.URL == "" || cfg.Key == "" {
		return nil, fmt.Errorf("%w: %s and %s must be set together", ErrAPIConfig, EnvAPIURL, EnvAPIKey)
	}

	if cfg.Model == "" {
		cfg.Model = DefaultAPIModel
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Minute
	}

	if len(cfg.Backoff) == 0 {
		cfg.Backoff = []time.Duration{2 * time.Second, 8 * time.Second, 20 * time.Second}
	}

	endpoint, err := outbound.ParseEndpoint(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAPIConfig, err)
	}

	client, err := outbound.TranscribeAPI(cfg.URL, cfg.Timeout, cfg.Test)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAPIConfig, err)
	}

	return &API{cfg: cfg, endpoint: endpoint, client: client}, nil
}

// Name implements Provider.
func (*API) Name() string { return ProviderAPI }

// Model implements Provider.
func (a *API) Model() string { return a.cfg.Model }

// Host is the host that receives the audio; the job message names it.
func (a *API) Host() string { return a.endpoint.Host }

// UploadMessage is the message of the job while a part is sent.
func (a *API) UploadMessage(part, parts int) string {
	return fmt.Sprintf("Uploading part %d of %d to %s", part, parts, a.Host())
}

// Check never downloads and never dials: a configured provider can run.
func (*API) Check(context.Context) error { return nil }

// redact removes the key from text that may reach an error, a log or a job message.
func (a *API) redact(s string) string {
	if a.cfg.Key == "" {
		return s
	}

	return strings.ReplaceAll(s, a.cfg.Key, "[redacted]")
}

// Transcribe cuts the audio in parts of at most ten minutes at silences, sends them one at a time and
// joins the words, shifted by the start of each part.
func (a *API) Transcribe(ctx context.Context, in Input, progress func(done, total time.Duration)) (Raw, error) {
	lang, err := Language(in.Language)
	if err != nil {
		return Raw{}, err
	}

	if in.WorkDir == nil || a.cfg.FFmpeg == nil {
		return Raw{}, failure(ReasonPrepare, "The transcription job is incomplete.",
			errors.New("run directory and FFmpeg are required"), "")
	}

	total := in.Duration.Milliseconds()
	bounds := chunkBounds(total, in.Silences)
	raw := Raw{Info: map[string]string{"provider": ProviderAPI, "model": a.cfg.Model, "host": a.Host()}}

	for k := range len(bounds) - 1 {
		start, end := bounds[k], bounds[k+1]

		wav, err := in.WorkDir.Path("part-" + strconv.Itoa(k) + ".wav")
		if err != nil {
			return Raw{}, failure(ReasonPrepare, "The work folder is not usable.", err, "")
		}

		if err := extractWAV(ctx, a.cfg.FFmpeg, in, wav, start, end-start); err != nil {
			return Raw{}, err
		}

		audio, err := os.ReadFile(wav)
		if err != nil || len(audio) > maxChunkBytes {
			return Raw{}, failure(ReasonPrepare, "A part of the audio is not usable.", err, "")
		}

		part, err := a.send(ctx, audio, lang)
		if err != nil {
			return Raw{}, err
		}

		if k == 0 || raw.Language == "" {
			raw.Language = part.Language
		}

		for _, w := range part.Words {
			w.StartMS += start
			w.EndMS += start
			raw.Words = append(raw.Words, w)
		}

		if progress != nil {
			progress(time.Duration(end)*time.Millisecond, in.Duration)
		}
	}

	return raw, nil
}

// chunkBounds are the cut points, in milliseconds, of a part of at most chunkMS: near each multiple of the
// part length, the middle of the silence closest to it within 30 s, else the multiple itself.
func chunkBounds(total int64, silences []Range) []int64 {
	bounds := []int64{0}

	for at := int64(chunkMS); at < total; at += chunkMS {
		cut, best := at, int64(chunkSearchMS+1)

		for _, s := range silences {
			mid := (s.StartMS + s.EndMS) / 2
			if d := abs64(mid - at); d <= chunkSearchMS && d < best && mid > bounds[len(bounds)-1] && mid < total {
				cut, best = mid, d
			}
		}

		bounds = append(bounds, cut)
	}

	return append(bounds, max(total, 1))
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}

	return v
}

type apiWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type apiSegment struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	AvgLogprob float64 `json:"avg_logprob"`
}

type apiResponse struct {
	Language string        `json:"language"`
	Words    *[]apiWord    `json:"words"`
	Segments []apiSegment  `json:"segments"`
	Error    *apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Message string `json:"message"`
}

// send posts one part, with the retries of SDD-011 section 10.3, and reads the words.
func (a *API) send(ctx context.Context, audio []byte, lang string) (Raw, error) {
	body, contentType, err := multipartBody(audio, a.cfg.Model, lang)
	if err != nil {
		return Raw{}, failure(ReasonPrepare, "The request could not be made.", err, "")
	}

	var lastErr error

	for attempt := 0; ; attempt++ {
		raw, wait, retry, err := a.once(ctx, body, contentType)
		if err == nil {
			return raw, nil
		}

		lastErr = err
		if !retry || attempt >= len(a.cfg.Backoff) {
			return Raw{}, lastErr
		}

		if wait <= 0 {
			wait = a.cfg.Backoff[attempt]
		}

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return Raw{}, ctx.Err()
		}
	}
}

// multipartBody is the request body: the audio under the fixed file name and the fixed fields, nothing
// of the project, the asset or the machine.
func multipartBody(audio []byte, model, lang string) ([]byte, string, error) {
	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, apiFileName))
	h.Set("Content-Type", "audio/wav")

	fw, err := mw.CreatePart(h)
	if err != nil {
		return nil, "", err
	}

	if _, err := fw.Write(audio); err != nil {
		return nil, "", err
	}

	fields := [][2]string{
		{"model", model},
		{"response_format", "verbose_json"},
		{"temperature", "0"},
		{"timestamp_granularities[]", "word"},
		{"timestamp_granularities[]", "segment"},
	}
	if lang != "auto" {
		fields = append(fields, [2]string{"language", lang})
	}

	for _, f := range fields {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			return nil, "", err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), mw.FormDataContentType(), nil
}

// once makes one request. retry says whether the failure is worth another attempt; wait is the
// Retry-After of the server when it is at most 60 s.
func (a *API) once(
	ctx context.Context,
	body []byte,
	contentType string,
) (raw Raw, wait time.Duration, retry bool, err error) {
	target := strings.TrimRight(a.endpoint.String(), "/") + "/audio/transcriptions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return Raw{}, 0, false, failure(
			ReasonAPIError,
			"The request could not be made.",
			errors.New(a.redact(err.Error())),
			"",
		)
	}

	req.Header.Set("Authorization", "Bearer "+a.cfg.Key)
	req.Header.Set("Content-Type", contentType)

	res, err := a.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Raw{}, 0, false, ctx.Err()
		}

		cause := errors.New(a.redact(urlErrorText(err)))

		return Raw{}, 0, !isCertError(
				err,
			), failure(
				ReasonAPIUnreachable,
				"The transcription server could not be reached.",
				cause,
				"",
			)
	}

	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(res.Body, maxAPIResponse+1))
	if err != nil {
		return Raw{}, 0, true, failure(
			ReasonAPIUnreachable,
			"The transcription response was interrupted.",
			errors.New(a.redact(err.Error())),
			"",
		)
	}

	if len(data) > maxAPIResponse {
		return Raw{}, 0, false, failure(ReasonAPITooLarge, "The transcription response is too large.", nil, "")
	}

	return a.answer(res, data)
}

func (a *API) answer(res *http.Response, data []byte) (Raw, time.Duration, bool, error) {
	switch code := res.StatusCode; {
	case code >= 300 && code < 400:
		return Raw{}, 0, false, failure(
			ReasonAPIRedirect,
			"The transcription server answered with a redirect, which is refused.",
			nil,
			"",
		)
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return Raw{}, 0, false, unavailable(ReasonAPIUnauthorized, "The transcription server refused the key.", nil,
			map[string]any{"status": code})
	case code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500:
		return Raw{}, retryAfter(res), true, a.apiFailure(code, data)
	case code != http.StatusOK:
		return Raw{}, 0, false, a.apiFailure(code, data)
	}

	raw, err := a.parseAPI(data)

	return raw, 0, false, err
}

func (a *API) apiFailure(status int, data []byte) *ffmpeg.Error {
	e := failure(ReasonAPIError, "The transcription server answered with an error.", nil, "")
	e.Details["status"] = status

	var body apiResponse
	if depth(data) <= maxJSONDepth && json.Unmarshal(data, &body) == nil && body.Error != nil {
		e.Details["message"] = truncate(cleanText(a.redact(body.Error.Message)), 200)
	}

	return e
}

func retryAfter(res *http.Response) time.Duration {
	secs, err := strconv.Atoi(res.Header.Get("Retry-After"))
	if err != nil || secs < 0 || secs > 60 {
		return 0
	}

	return time.Duration(secs) * time.Second
}

// parseAPI reads the verbose JSON: words in seconds become integer milliseconds, and the confidence of a
// word is exp(avg_logprob) of the segment that holds its start.
func (a *API) parseAPI(data []byte) (Raw, error) {
	if depth(data) > maxJSONDepth {
		return Raw{}, failure(ReasonAPIError, "The transcription response is nested too deeply.", nil, "")
	}

	var doc apiResponse
	if err := json.Unmarshal(data, &doc); err != nil {
		return Raw{}, failure(
			ReasonAPIError,
			"The transcription response could not be read.",
			errors.New(a.redact(err.Error())),
			"",
		)
	}

	if doc.Words == nil {
		e := failure(ReasonAPINoWords, "The server returned no word timestamps.", nil, "")
		e.Hint = "Use a model that returns word timestamps (for example whisper-1) in " + EnvAPIModel + "."

		return Raw{}, e
	}

	raw := Raw{Language: languageCode(cleanText(doc.Language))}

	for _, w := range *doc.Words {
		text := strings.TrimSpace(cleanText(w.Word))
		if text == "" {
			continue
		}

		start := int64(math.Round(w.Start * 1000))
		raw.Words = append(raw.Words, RawWord{
			Text: text, StartMS: start, EndMS: max(int64(math.Round(w.End*1000)), start),
			Confidence: segmentConfidence(doc.Segments, w.Start),
		})
	}

	return raw, nil
}

func segmentConfidence(segs []apiSegment, at float64) float64 {
	for _, s := range segs {
		if at >= s.Start && at < s.End {
			return math.Round(math.Exp(s.AvgLogprob)*1000) / 1000
		}
	}

	return math.NaN()
}

// urlErrorText is the text of a transport error without the URL that *url.Error adds.
func urlErrorText(err error) string {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err.Error()
	}

	return err.Error()
}

func isCertError(err error) bool {
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}

	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return true
	}

	_, ok := errors.AsType[x509.CertificateInvalidError](err)

	return ok
}
