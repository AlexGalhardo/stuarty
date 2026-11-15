// Command fakewhisper stands in for whisper-cli in the tests of engine/transcribe. It reads the options
// of the fixed template (-f, -of), and its behaviour comes from FAKEWHISPER_MODE:
//
//	ok       prints progress lines on standard output and copies FAKEWHISPER_CANNED to <prefix>.json
//	corrupt  prints an error on standard error, writes nothing and exits 0 (what whisper-cli does)
//	empty    writes an empty <prefix>.json and exits 0
//	fail     exits 3 with no output
package main

import (
	"fmt"
	"os"
)

func main() {
	var wav, prefix string
	for i := 1; i+1 < len(os.Args); i++ {
		switch os.Args[i] {
		case "-f":
			wav = os.Args[i+1]
		case "-of":
			prefix = os.Args[i+1]
		}
	}
	switch os.Getenv("FAKEWHISPER_MODE") {
	case "corrupt":
		fmt.Fprintf(os.Stderr, "error: failed to read audio file '%s'\n", wav)
	case "empty":
		_ = os.WriteFile(prefix+".json", nil, 0o600)
	case "fail":
		fmt.Fprintln(os.Stderr, "error: failed to load model")
		os.Exit(3)
	default:
		fmt.Fprintf(os.Stderr, "whisper_print_progress_callback: progress = 10%%\n")
		fmt.Println("[00:00:00.000 --> 00:00:01.000]   Hello")
		fmt.Println("[00:00:01.000 --> 00:00:03.500]   world")
		data, err := os.ReadFile(os.Getenv("FAKEWHISPER_CANNED"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: no canned output")
			os.Exit(2)
		}
		if err := os.WriteFile(prefix+".json", data, 0o600); err != nil {
			os.Exit(2)
		}
	}
}
