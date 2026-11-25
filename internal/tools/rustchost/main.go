// Command rustchost is the RUSTC_WRAPPER of scripts/build-renderer.sh on a machine that is not an x86-64
// Linux one (epic 93). It runs the compiler it is given and changes one thing: the "host:" line of
// `rustc -vV`, which it makes the host of the Linux runner.
//
// Why: cargo hashes the whole text of `rustc -vV` into the `-C metadata` of every crate it builds, and
// that value is the disambiguator in the mangled symbol of every item of those crates. The linker lays
// out the functions and the panic locations in the order of their symbols, so a Windows build and a
// Linux build of the same source and the same toolchain differ in almost every function (the crates of
// the sysroot are prebuilt and agree), although nothing but that line differs. With the line made equal
// the two systems build the same module byte for byte.
//
// Only the version query is rewritten; every other call passes through with its arguments, streams and
// exit status. The proc macros and build scripts still run on the real host.
//
//	rustchost rustc -vV
//	rustchost rustc --crate-name x ...
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// linuxHost is the host triple of the runner the module is released from (ubuntu-24.04).
const linuxHost = "x86_64-unknown-linux-gnu"

// normalize rewrites the "host:" line of the output of `rustc -vV`.
func normalize(out []byte) []byte {
	lines := bytes.SplitAfter(out, []byte("\n"))
	for i, line := range lines {
		if !bytes.HasPrefix(line, []byte("host: ")) {
			continue
		}
		ending := line[len(bytes.TrimRight(line, "\r\n")):]
		lines[i] = append([]byte("host: "+linuxHost), ending...)
	}
	return bytes.Join(lines, nil)
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "rustchost: usage: rustchost <rustc> [arguments]")
		return 2
	}
	//nolint:gosec // G204: runs the compiler cargo names
	cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	if len(args) == 2 && args[1] == "-vV" {
		out, err := cmd.Output()
		if err != nil {
			return exitCode(err)
		}
		if _, err := os.Stdout.Write(normalize(out)); err != nil {
			return 1
		}
		return 0
	}
	cmd.Stdout = os.Stdout
	return exitCode(cmd.Run())
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "rustchost:", err)
	return 1
}
