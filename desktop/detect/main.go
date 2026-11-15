// Command detect serves the feature-detection page of SDD-012 section 21.2: the
// page, the proxy fixture (with Range), a server-sent-event stream for the
// EventSource test and a route that stores the results. It is a tool for the
// choice of the desktop shell (ADR-0023); nothing in the product imports it.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed index.html detect.js detect.css fixtures
var site embed.FS

// csp is the default content security policy of the product, without the nonce
// the product adds: the page has no inline script and no inline style.
const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; " +
	"frame-ancestors 'none'; form-action 'self'"

const (
	events    = 100
	dropAfter = 40
	interval  = 10 * time.Millisecond
	maxBody   = 1 << 20
)

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

// handler builds the routes; resultsDir is where POST /results writes and port
// is the port the server listens on, which the Host header must name (a page
// served under another name, as in DNS rebinding, is refused).
func handler(resultsDir, port string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(site))
	mux.HandleFunc("/events", serveEvents)
	mux.HandleFunc("POST /results", func(w http.ResponseWriter, r *http.Request) {
		saveResults(w, r, resultsDir)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, port) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

// serveEvents emits events numbered 1 to 100. A connection that does not say
// where it resumes (no Last-Event-ID) is cut after the 40th event, so the test
// can check that the browser reconnects with Last-Event-ID 40. Every event
// carries the Last-Event-ID the connection started with.
func serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	resumed := r.Header.Get("Last-Event-ID")
	first := 1

	if n, err := strconv.Atoi(resumed); err == nil {
		first = n + 1
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	for id := first; id <= events; id++ {
		if resumed == "" && id > dropAfter {
			return
		}

		payload, err := json.Marshal(struct {
			ID          int    `json:"id"`
			ResumedFrom string `json:"resumed_from"`
		}{id, resumed})
		if err != nil {
			return
		}

		// The data is the JSON of two fields, in a text/event-stream.
		fmt.Fprintf(w, "id: %d\ndata: %s\n\n", id, payload)
		flusher.Flush()

		select {
		case <-r.Context().Done():
			return
		case <-time.After(interval):
		}
	}
}

// hostAllowed accepts only the two names of the loopback server.
func hostAllowed(host, port string) bool {
	return host == "127.0.0.1:"+port || host == "localhost:"+port
}

// sameOrigin is true for a request that the page of this server made: the
// Origin header names this server, or, when there is none, the browser says
// Sec-Fetch-Site: same-origin. A request with a different Origin is refused
// even when the other header says same-origin; one with neither is refused.
func sameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin == "http://"+r.Host
	}

	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}

// saveResults writes the posted JSON to <platform>-<webview>-<date>.json, or
// to the same name with a counter when that file exists. Errors are generic.
func saveResults(w http.ResponseWriter, r *http.Request, dir string) {
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// application/json is not a type a form or a no-cors request can send, so
	// a cross-site page needs a preflight, which this server never answers.
	if mediaType, _, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	); err != nil ||
		mediaType != "application/json" {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad request", http.StatusRequestEntityTooLarge)
		return
	}

	var head struct {
		Platform string `json:"platform"`
		Webview  string `json:"webview"`
	}

	if err := json.Unmarshal(body, &head); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	name, err := writeNew(dir, clean(head.Platform), clean(head.Webview), body)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	reply, err := json.Marshal(map[string]string{"file": name})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(reply)
}

// maxCopies is how many files of one name a day may have.
const maxCopies = 1000

// writeNew creates the file with O_EXCL, so an existing file is never
// overwritten: the name gets -2, -3 and so on.
func writeNew(dir, platform, webview string, body []byte) (string, error) {
	base := fmt.Sprintf("%s-%s-%s", platform, webview, time.Now().Format("2006-01-02"))

	for n := 1; n <= maxCopies; n++ {
		name := base + ".json"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.json", base, n)
		}

		// The name is built from clean() parts, so it is one plain element; the
		// check keeps a future change from turning it into a path.
		if filepath.Base(name) != name || strings.Contains(name, "..") {
			return "", errors.New("unsafe result name")
		}

		f, err := openInRoot(dir, name)
		if errors.Is(err, fs.ErrExist) {
			continue
		}

		if err != nil {
			return "", err
		}

		_, werr := f.Write(body)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}

		return name, werr
	}

	return "", errors.New("too many files")
}

func clean(s string) string {
	s = strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(s), "-"), ".-")
	if s == "" {
		return "unknown"
	}

	if len(s) > 48 {
		s = s[:48]
	}

	return s
}

func main() {
	addr := flag.String("addr", "127.0.0.1:7979", "address to listen on (loopback only)")
	results := flag.String("results", "desktop/results", "folder for the results files")
	flag.Parse()

	log.Printf("detect: http://%s/", *addr)

	_, port, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{Addr: *addr, Handler: handler(*results, port), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// openInRoot creates name inside dir through os.Root, so no name can leave it.
func openInRoot(dir, name string) (*os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}

	defer func() { _ = root.Close() }()

	return root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}
