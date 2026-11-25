package main

import "testing"

func TestNormalizeRewritesOnlyTheHostLine(t *testing.T) {
	in := "rustc 1.100.0 (0ef1bc386 2026-11-09)\nbinary: rustc\ncommit-hash: 0ef1bc3\nhost: x86_64-pc-windows-msvc\nrelease: 1.100.0\nLLVM version: 23.1.5\n"
	want := "rustc 1.100.0 (0ef1bc386 2026-11-09)\nbinary: rustc\ncommit-hash: 0ef1bc3\nhost: x86_64-unknown-linux-gnu\nrelease: 1.100.0\nLLVM version: 23.1.5\n"
	if got := string(normalize([]byte(in))); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestNormalizeKeepsTheLineEnding(t *testing.T) {
	got := string(normalize([]byte("host: aarch64-apple-darwin\r\nrelease: 1.100.0\r\n")))
	if want := "host: x86_64-unknown-linux-gnu\r\nrelease: 1.100.0\r\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeIsTheIdentityOnTheLinuxHost(t *testing.T) {
	in := "binary: rustc\nhost: x86_64-unknown-linux-gnu\nrelease: 1.100.0\n"
	if got := string(normalize([]byte(in))); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeWithoutAHostLine(t *testing.T) {
	in := "binary: rustc\nrelease: 1.100.0"
	if got := string(normalize([]byte(in))); got != in {
		t.Fatalf("got %q", got)
	}
}
