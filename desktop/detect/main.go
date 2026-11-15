// Command detect serves the feature-detection page of SDD-012 section 21.2: the
// page, the proxy fixture (with Range), a server-sent-event stream for the
// EventSource test and a route that stores the results. It is a tool for the
// choice of the desktop shell (ADR-0023); nothing in the product imports it.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
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

// handler builds the routes; resultsDir is where POST /results writes.
func handler(resultsDir string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(site))
	mux.HandleFunc("/events", serveEvents)
	mux.HandleFunc("POST /results", func(w http.ResponseWriter, r *http.Request) {
		saveResults(w, r, resultsDir)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// saveResults writes the posted JSON to <platform>-<webview>-<date>.json.
func saveResults(w http.ResponseWriter, r *http.Request, dir string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	var head struct {
		Platform string `json:"platform"`
		Webview  string `json:"webview"`
	}

	if err := json.Unmarshal(body, &head); err != nil {
		http.Error(w, "not JSON", http.StatusBadRequest)
		return
	}

	name := fmt.Sprintf("%s-%s-%s.json", clean(head.Platform), clean(head.Webview), time.Now().Format("2006-01-02"))

	if err := os.MkdirAll(dir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reply, err := json.Marshal(map[string]string{"file": name})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(reply)
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

	srv := &http.Server{Addr: *addr, Handler: handler(*results), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
