// Command fakeserver stands in for `vukem-video serve` in the shell tests. The
// variable FAKE_MODE picks the behaviour: ok (default), never-ready (the
// runtime file is written but /up answers 503), exit (fails with a message),
// ignore-term (does not stop on SIGTERM).
package main

import (
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
