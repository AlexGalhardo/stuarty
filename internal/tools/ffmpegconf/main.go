// Command ffmpegconf checks that a FFmpeg configure line is LGPL safe (D-017, ADR-0011, NFR-043):
// no --enable-gpl, no --enable-nonfree, none of the GPL or nonfree libraries, no OpenH264 compiled
// in, and no --enable-* flag outside the allowlist. The line is the flag file of the build
// (.config/ffmpeg-build/configure-flags.txt) or the `configuration:` line of a built binary.
//
//	go run ./internal/tools/ffmpegconf -allow .config/ffmpeg-build/configure-allowlist.txt -flags .config/ffmpeg-build/configure-flags.txt
//	ffmpeg -version | go run ./internal/tools/ffmpegconf -allow .config/ffmpeg-build/configure-allowlist.txt -line -
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// forbidden maps a flag to why it is refused; the allowlist alone would refuse them too, this
// gives the reason.
var forbidden = map[string]string{
	"--enable-gpl":           "GPL build",
	"--enable-nonfree":       "nonfree build",
	"--enable-libopenh264":   "OpenH264 must be loaded at run time, never compiled in (D-017)",
	"--enable-libx264":       "x264 is GPL",
	"--enable-libx265":       "x265 is GPL",
	"--enable-libxvid":       "Xvid is GPL",
	"--enable-libfdk-aac":    "fdk-aac is nonfree",
	"--enable-libvidstab":    "vid.stab is GPL",
	"--enable-librubberband": "Rubber Band is GPL",
	"--enable-frei0r":        "frei0r is GPL",
	"--enable-libcdio":       "libcdio is GPL",
	"--enable-libzvbi":       "libzvbi is GPL",
	"--enable-libsmbclient":  "libsmbclient is GPL",
	"--enable-libdavs2":      "davs2 is GPL",
	"--enable-libxavs2":      "xavs2 is GPL",
	"--enable-libaribb24":    "aribb24 is LGPL with a patent notice the project does not take",
	"--enable-decklink":      "DeckLink is nonfree",
	"--enable-openssl":       "OpenSSL needs a nonfree or version3 combination the project does not take",
	"--enable-libnpp":        "NVIDIA NPP is nonfree",
	"--enable-cuda-nvcc":     "CUDA is nonfree",
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ffmpegconf", flag.ContinueOnError)
	fs.SetOutput(stderr)

	allowPath := fs.String(
		"allow",
		".config/ffmpeg-build/configure-allowlist.txt",
		"the file of allowed --enable-* flags",
	)
	flagsPath := fs.String("flags", "", "the flag file of the build (sections and one flag per line)")
	linePath := fs.String(
		"line",
		"",
		"a file with a configure line or the output of ffmpeg -version; - is standard input",
	)

	if err := fs.Parse(argv); err != nil {
		return 2
	}

	if (*flagsPath == "") == (*linePath == "") {
		fmt.Fprintln(stderr, "ffmpegconf: give exactly one of -flags and -line")

		return 2
	}

	allowText, err := os.ReadFile(*allowPath)
	if err != nil {
		fmt.Fprintf(stderr, "ffmpegconf: %v\n", err)

		return 1
	}

	var text []byte

	switch {
	case *flagsPath != "":
		text, err = os.ReadFile(*flagsPath)
		text = []byte(dropComments(string(text)))
	case *linePath == "-":
		text, err = io.ReadAll(stdin)
	default:
		text, err = os.ReadFile(*linePath)
	}

	if err != nil {
		fmt.Fprintf(stderr, "ffmpegconf: %v\n", err)

		return 1
	}

	problems := check(string(text), allowedFlags(string(allowText)))
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(stderr, "ffmpegconf: %s\n", p)
		}

		return 1
	}

	fmt.Fprintln(stdout, "ffmpegconf: the configure flags are LGPL safe")

	return 0
}

// dropComments removes the comment lines of a flag file, so that prose is not read as flags.
func dropComments(text string) string {
	var b strings.Builder

	for line := range strings.SplitSeq(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// allowedFlags reads the allowlist: one flag per line, # starts a comment.
func allowedFlags(text string) map[string]bool {
	allowed := map[string]bool{}

	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			allowed[line] = true
		}
	}

	return allowed
}

// check returns one message per refused --enable-* flag of text, sorted and without repeats.
// Only flags are looked at: --disable-* flags, paths and architecture options are not licence
// relevant, and a line that is a whole `configuration:` text is read as words.
func check(text string, allowed map[string]bool) []string {
	var problems []string

	for word := range strings.FieldsSeq(text) {
		if !strings.HasPrefix(word, "--enable-") {
			continue
		}

		name, _, _ := strings.Cut(word, "=")

		if why, bad := forbidden[name]; bad {
			problems = append(problems, fmt.Sprintf("%s is refused: %s", name, why))

			continue
		}

		if !allowed[name] {
			problems = append(problems, name+" is not in the allowlist")
		}
	}

	slices.Sort(problems)

	return slices.Compact(problems)
}
