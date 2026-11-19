package transcribe

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

const canaryKey = "sk-canary-0123456789abcdef"

const goodJSON = `{"language":"portuguese","duration":4.0,
 "words":[{"word":" Olá","start":0.5,"end":1.0},{"word":"mundo","start":1.0,"end":1.8}],
 "segments":[{"start":0.0,"end":4.0,"avg_logprob":-0.25}]}`

func fastAPI(t *testing.T, srvURL string, test *outbound.TestOptions) *API {
	t.Helper()

	a, err := NewAPI(APIConfig{
		URL: srvURL, Key: canaryKey, Backoff: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
		Test: test,
	})
	if err != nil {
		t.Fatal(err)
	}

	return a
}

func asReason(t *testing.T, err error) (string, *ffmpeg.Error) {
	t.Helper()

	fe, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok {
		t.Fatalf("err = %v, want an *ffmpeg.Error", err)
	}

	return fe.Reason, fe
}

// SDD-007 section 10.1 path 1: the body holds the audio under the fixed name and the fixed fields, and
// nothing of the project.
func TestRequestCarriesNoProjectData(t *testing.T) {
	var (
		gotAuth string
		parts   = map[string][]string{}
		fname   string
		audio   []byte
		raw     []byte
	)

	srv, test := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ = io.ReadAll(r.Body)

		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Error(err)
		}

		mr := multipart.NewReader(bytes.NewReader(raw), params["boundary"])

		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}

			b, _ := io.ReadAll(p)
			if p.FormName() == "file" {
				fname, audio = p.FileName(), b

				continue
			}

			parts[p.FormName()] = append(parts[p.FormName()], string(b))
		}

		_, _ = w.Write([]byte(goodJSON))
	}))

	a := fastAPI(t, srv.URL+"/v1", test)
	wav := append([]byte("RIFF....WAVEfmt "), bytes.Repeat([]byte{1}, 100)...)

	res, err := a.send(context.Background(), wav, "pt")
	if err != nil {
		t.Fatal(err)
	}

	if fname != "audio.wav" || !bytes.Equal(audio, wav) {
		t.Errorf("file %q, %d bytes", fname, len(audio))
	}

	want := map[string][]string{
		"model": {"whisper-1"}, "response_format": {"verbose_json"}, "temperature": {"0"}, "language": {"pt"},
		"timestamp_granularities[]": {"word", "segment"},
	}
	for k, v := range want {
		if !slices.Equal(parts[k], v) {
			t.Errorf("field %s = %v, want %v", k, parts[k], v)
		}
	}

	if len(parts) != len(want) {
		t.Errorf("fields %v: the body carries more than the fixed fields", parts)
	}

	if gotAuth != "Bearer "+canaryKey {
		t.Errorf("Authorization %q", gotAuth)
	}

	if bytes.Contains(raw, []byte(canaryKey)) {
		t.Error("the key is in the body")
	}

	if res.Language != "pt" || len(res.Words) != 2 || res.Words[0].Text != "Olá" || res.Words[0].StartMS != 500 {
		t.Errorf("parsed %+v", res)
	}

	if c := res.Words[0].Confidence; math.Abs(c-0.779) > 0.001 {
		t.Errorf("confidence %v, want exp(-0.25) = 0.779", c)
	}

	if a.Host() != strings.TrimPrefix(srv.URL, "https://") || !strings.Contains(a.UploadMessage(2, 5), a.Host()) {
		t.Errorf("the message must name the host: %q", a.UploadMessage(2, 5))
	}
}

func TestAPIAutoLanguageSendsNoLanguageField(t *testing.T) {
	var body []byte

	srv, test := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(goodJSON))
	}))

	if _, err := fastAPI(t, srv.URL, test).send(context.Background(), []byte("x"), "auto"); err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(body, []byte(`name="language"`)) {
		t.Error("a language was sent for auto")
	}
}

func TestAPIRefusals(t *testing.T) {
	var elsewhere atomic.Int32

	other, _ := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere.Add(1)
		_, _ = w.Write([]byte(goodJSON))
	}))

	big := bytes.Repeat([]byte("a"), 17<<20)

	var hits atomic.Int32

	srv, test := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// An unread request body makes net/http close the connection with a delay of 500 ms after the reply;
		// the client of the 17 MiB answer, slow under the race detector, then reads a connection reset.
		_, _ = io.Copy(io.Discard, r.Body)

		switch {
		case strings.HasSuffix(r.URL.Path, "/redirect/audio/transcriptions"):
			http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
		case strings.HasSuffix(r.URL.Path, "/big/audio/transcriptions"):
			_, _ = w.Write(big)
		case strings.HasSuffix(r.URL.Path, "/nowords/audio/transcriptions"):
			_, _ = w.Write([]byte(`{"language":"en","text":"hi"}`))
		case strings.HasSuffix(r.URL.Path, "/denied/audio/transcriptions"):
			w.WriteHeader(http.StatusUnauthorized)
		case strings.HasSuffix(r.URL.Path, "/leaky/audio/transcriptions"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"bad key ` + canaryKey + ` for you"}}`))
		case strings.HasSuffix(r.URL.Path, "/flaky/audio/transcriptions"):
			if hits.Load() < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)

				return
			}

			_, _ = w.Write([]byte(goodJSON))
		case strings.HasSuffix(r.URL.Path, "/down/audio/transcriptions"):
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	tests := []struct {
		path   string
		reason string
		hits   int32
	}{
		{"/redirect", ReasonAPIRedirect, 1},
		{"/big", ReasonAPITooLarge, 1},
		{"/nowords", ReasonAPINoWords, 1},
		{"/denied", ReasonAPIUnauthorized, 1},
		{"/leaky", ReasonAPIError, 1},
		{"/down", ReasonAPIError, 4},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			hits.Store(0)

			_, err := fastAPI(t, srv.URL+tt.path, test).send(context.Background(), []byte("x"), "auto")
			reason, fe := asReason(t, err)

			if reason != tt.reason {
				t.Errorf("reason %q, want %q", reason, tt.reason)
			}

			if hits.Load() != tt.hits {
				t.Errorf("%d requests, want %d", hits.Load(), tt.hits)
			}

			if strings.Contains(err.Error(), canaryKey) || strings.Contains(fmtDetails(fe), canaryKey) {
				t.Errorf("the key is in the error: %v %v", err, fe.Details)
			}
		})
	}

	if elsewhere.Load() != 0 {
		t.Errorf("the redirect was followed: %d requests to the other host", elsewhere.Load())
	}

	t.Run("retries then succeeds", func(t *testing.T) {
		hits.Store(0)

		res, err := fastAPI(t, srv.URL+"/flaky", test).send(context.Background(), []byte("x"), "auto")
		if err != nil || len(res.Words) != 2 || hits.Load() != 3 {
			t.Errorf("%v, %d words, %d requests", err, len(res.Words), hits.Load())
		}
	})
}

func fmtDetails(fe *ffmpeg.Error) string {
	var sb strings.Builder

	for k, v := range fe.Details {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(strings.TrimSpace(strings.ReplaceAll(strings.Join([]string{asString(v)}, ""), "\n", " ")))
		sb.WriteString(";")
	}

	return sb.String()
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}

	return ""
}

func TestAPIInvalidCertificateIsRefused(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(goodJSON))
	}))
	defer srv.Close()

	// No test trust: the self-signed certificate of the server must fail the verification, and the
	// failure is not retried.
	a, err := NewAPI(APIConfig{URL: srv.URL, Key: canaryKey, Backoff: []time.Duration{time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.send(context.Background(), []byte("x"), "auto")
	if reason, _ := asReason(t, err); reason != ReasonAPIUnreachable || hits.Load() != 0 {
		t.Errorf("reason %q, %d requests reached the handler", reason, hits.Load())
	}

	if strings.Contains(err.Error(), canaryKey) {
		t.Error("the key is in the error")
	}
}

func TestAPIHTTPIsOnlyForLoopback(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(goodJSON))
	}))
	defer plain.Close()

	if _, err := fastAPI(t, plain.URL, nil).send(context.Background(), []byte("x"), "auto"); err != nil {
		t.Errorf("http to a loopback host: %v", err)
	}

	for name, cfg := range map[string]APIConfig{
		"http to a public host": {URL: "http://api.example.com/v1", Key: "k"},
		"userinfo":              {URL: "https://user:pw@api.example.com/v1", Key: "k"},
		"query":                 {URL: "https://api.example.com/v1?x=1", Key: "k"},
		"other scheme":          {URL: "ftp://api.example.com/v1", Key: "k"},
		"key without a URL":     {Key: "k"},
		"URL without a key":     {URL: "https://api.example.com/v1"},
		"not a URL":             {URL: "::::", Key: "k"},
	} {
		if _, err := NewAPI(cfg); !errors.Is(err, ErrAPIConfig) {
			t.Errorf("%s: err = %v, want the unsafe endpoint error", name, err)
		}
	}
}

// With the variables unset the provider is not constructed, and nothing dials.
func TestAPIUnsetIsNotConstructed(t *testing.T) {
	var dials int

	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error {
		dials++

		return errors.New("no connection")
	})
	defer restore()

	a, err := APIFromEnv(func(string) string { return "" }, nil)
	if a != nil || err != nil || dials != 0 {
		t.Errorf("provider %v, err %v, %d dials", a, err, dials)
	}

	if _, err := APIFromEnv(func(k string) string {
		if k == EnvAPIKey {
			return "k"
		}

		return ""
	}, nil); !errors.Is(err, ErrAPIConfig) {
		t.Errorf("a key without a URL: %v", err)
	}
}

func TestChunkBounds(t *testing.T) {
	min10 := int64(chunkMS)

	if got := chunkBounds(300_000, nil); !slices.Equal(got, []int64{0, 300_000}) {
		t.Errorf("short audio: %v", got)
	}

	if got := chunkBounds(min10, nil); !slices.Equal(got, []int64{0, min10}) {
		t.Errorf("exactly one part: %v", got)
	}

	// 25 minutes: cuts near 10 and 20 minutes; the first at the middle of a silence 12 s early, the
	// second at the multiple because the nearest silence is 45 s away.
	sil := []Range{
		{StartMS: min10 - 13_000, EndMS: min10 - 11_000},
		{StartMS: 2*min10 + 44_000, EndMS: 2*min10 + 46_000},
	}
	if got, want := chunkBounds(
		25*60_000,
		sil,
	), []int64{
		0,
		min10 - 12_000,
		2 * min10,
		25 * 60_000,
	}; !slices.Equal(
		got,
		want,
	) {
		t.Errorf("bounds %v, want %v", got, want)
	}
}

func TestParseAPIClampsAndMapsLanguage(t *testing.T) {
	a := fastAPI(t, "https://api.example.com/v1", nil)

	raw, err := a.parseAPI([]byte(`{"language":"English","words":[{"word":"hi","start":0.0004,"end":0.3}]}`))
	if err != nil || raw.Language != "en" || raw.Words[0].EndMS != 300 || !math.IsNaN(raw.Words[0].Confidence) {
		t.Errorf("%+v, %v", raw, err)
	}

	if _, err := a.parseAPI([]byte(strings.Repeat("[", 40) + strings.Repeat("]", 40))); err == nil {
		t.Error("a deeply nested response was accepted")
	}
}

// tlsServer serves the handler over TLS on loopback and returns the options that trust it.
func tlsServer(t *testing.T, h http.Handler) (*httptest.Server, *outbound.TestOptions) {
	t.Helper()

	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	return srv, &outbound.TestOptions{RootCAs: pool}
}
