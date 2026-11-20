// Command firstflowfake plays, on loopback, the two outside services of the first complete flow
// (epic 38): an AI provider that speaks the OpenAI chat protocol, and a social network that
// speaks the protocol of the fake connector of service/social/fake. Nothing here reaches the
// network: the scripted run (scripts/first-flow.sh) and the agent harness start it next to a
// Stuarty server and tell that server its addresses.
//
// Both listen only on loopback. The social side records every upload, checks that the file the
// upload names exists, and lists what it received at GET /_uploads, so a run can prove that nothing
// went out before a person approved it, and that the file that went out is the exported one.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Model is the one model the fake provider offers.
const Model = "fake-small"

// ClientSecret is the client secret the fake network accepts at its token address.
const ClientSecret = "first-flow-client-secret"

// Draft is what the fake provider answers to every chat request: a plain script.
const Draft = "Sourdough, from starter to slice. Feed the starter, fold the dough, bake it hot. " +
	"Three days of waiting for one loaf, and it is worth every hour."

type upload struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	Visibility string `json:"visibility"`
	URL        string `json:"url"`
}

type network struct {
	mu      sync.Mutex
	uploads []upload
	base    string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "firstflowfake:", err)
		os.Exit(1)
	}
}

func run() error {
	providerAddr := flag.String("provider-addr", "127.0.0.1:0", "address of the fake AI provider, on loopback")
	socialAddr := flag.String("social-addr", "127.0.0.1:0", "address of the fake social network, on loopback")
	flag.Parse()

	provider, err := listenLoopback(*providerAddr)
	if err != nil {
		return err
	}

	social, err := listenLoopback(*socialAddr)
	if err != nil {
		return err
	}

	fake := &network{base: "http://" + social.Addr().String()}

	go serve(provider, providerMux())
	go serve(social, fake.mux())

	// One line, so a script can wait for it and read both addresses.
	fmt.Printf("ready provider=http://%s/v1 social=%s\n", provider.Addr(), fake.base)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	return nil
}

func listenLoopback(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("address %q: %w", addr, err)
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("address %q is not on loopback", addr)
	}

	return (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
}

func serve(l net.Listener, h http.Handler) {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "firstflowfake:", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func providerMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": []map[string]string{{"id": Model}}})
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
		writeJSON(w, http.StatusOK, map[string]any{
			"model":   Model,
			"choices": []map[string]any{{"message": map[string]string{"content": Draft}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 120, "completion_tokens": 40},
		})
	})

	return mux
}

func (n *network) mux() http.Handler {
	mux := http.NewServeMux()
	// The consent screen sends the browser straight back with a code.
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(r.URL.Query().Get("redirect_uri"))
		host := strings.ToLower(target.Hostname())
		if err != nil || target.Scheme != "http" || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
			http.Error(w, "bad redirect_uri", http.StatusBadRequest)

			return
		}

		q := target.Query()
		q.Set("code", "first-flow-code")
		q.Set("state", r.URL.Query().Get("state"))
		target.RawQuery = q.Encode()
		//nolint:gosec // G710: a fake consent screen on loopback; it answers the way the real one does
		http.Redirect(w, r, target.String(), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("client_secret") != ClientSecret {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_client"})

			return
		}

		//nolint:gosec // G101: throwaway values of a fake network, not credentials
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": "first-flow-access", "refresh_token": "first-flow-refresh", "expires_in": 3600,
		})
	})
	mux.HandleFunc("GET /account", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"id": "UC-FIRST-FLOW", "title": "First flow channel"})
	})
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{})
	})
	mux.HandleFunc("POST /videos", n.upload)
	mux.HandleFunc("GET /videos/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"state": "published"})
	})
	mux.HandleFunc("GET /_uploads", func(w http.ResponseWriter, _ *http.Request) {
		n.mu.Lock()
		defer n.mu.Unlock()

		writeJSON(w, http.StatusOK, map[string]any{"uploads": append([]upload{}, n.uploads...)})
	})

	return mux
}

// upload records a post. The file it names must exist and not be empty: a post whose video is not
// the exported file is a failed flow, and the network says so the way a real one would.
func (n *network) upload(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title      string `json:"title"`
		Visibility string `json:"visibility"`
		Path       string `json:"path"`
	}

	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body"})

		return
	}

	if strings.TrimSpace(in.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no video file at " + in.Path})

		return
	}

	cleanPath := filepath.Clean(in.Path)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no video file at " + in.Path})

		return
	}

	baseDir, err := filepath.Abs(".")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})

		return
	}

	rel, err := filepath.Rel(baseDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no video file at " + in.Path})

		return
	}

	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() || info.Size() == 0 || !strings.HasSuffix(strings.ToLower(absPath), ".mp4") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no video file at " + in.Path})

		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	id := fmt.Sprintf("ff%d", len(n.uploads)+1)
	u := upload{
		ID: id, Title: in.Title, Path: absPath, Bytes: info.Size(), Visibility: in.Visibility,
		URL: "https://fake.example/watch/" + id,
	}
	n.uploads = append(n.uploads, u)

	writeJSON(w, http.StatusOK, map[string]string{"id": id, "url": u.URL, "state": "published"})
}
