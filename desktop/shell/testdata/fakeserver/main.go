// Command fakeserver stands in for `vukem-video serve` in the shell tests. The
// variable FAKE_MODE picks the behaviour: ok (default), never-ready (the
// runtime file is written but /up answers 503), exit (fails with a message),
// ignore-term (does not stop on SIGTERM), impostor (answers the handshake with
// a proof of another token).
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func main() {
	mode := os.Getenv("FAKE_MODE")
	if mode == "exit" {
		fmt.Fprintln(os.Stderr, "fake: cannot open the workspace")
		os.Exit(3)
	}
	addr := os.Getenv("VUKEM_VIDEO_ADDR")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake:", err)
		os.Exit(1)
	}
	dir := workspace.Dir(os.Getenv("VUKEM_VIDEO_HOME"))
	if _, err := dir.EnsureToken(); err != nil {
		fmt.Fprintln(os.Stderr, "fake:", err)
		os.Exit(1)
	}
	rt := workspace.Runtime{PID: os.Getpid(), Addr: ln.Addr().String(), Mode: workspace.ModeLocal, Version: "0.0.0-fake"}
	if err := dir.WriteRuntime(rt); err != nil {
		fmt.Fprintln(os.Stderr, "fake:", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/up", func(w http.ResponseWriter, _ *http.Request) {
		if mode == "never-ready" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	token, _ := dir.LoadToken()
	mux.HandleFunc("/api/v1/handshake", func(w http.ResponseWriter, r *http.Request) {
		key := token
		if mode == "impostor" {
			key = "not the token"
		}
		m := hmac.New(sha256.New, []byte(key))
		m.Write([]byte("vukem-video-handshake-v1" + r.URL.Query().Get("nonce")))
		_ = json.NewEncoder(w).Encode(map[string]string{"proof": hex.EncodeToString(m.Sum(nil))})
	})
	mux.HandleFunc("/api/v1/system/launch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// The shell must prove the secret it put in this process environment (SDD-007 section 5.3).
		if want := os.Getenv("VUKEM_VIDEO_LAUNCHER_SECRET"); want == "" || r.Header.Get("Vukem-Video-Launcher") != want {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"url": "http://" + ln.Addr().String() + "/auth/launch?code=abc"})
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}

	sig := make(chan os.Signal, 1)
	if mode == "ignore-term" {
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
		go func() {
			<-sig
			_ = dir.RemoveRuntime()
			_ = srv.Close()
		}()
	}
	_ = srv.Serve(ln)
}
