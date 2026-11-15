// Command ffmpegfresh decides whether the FFmpeg that Vukem Video distributes is stale
// (SDD-007 section 12, "FFmpeg freshness"). It reads the pin in .config/ffmpeg.env and the inputs
// of the own build in .config/ffmpeg-build/sources.env, compares the patch release of the active
// source with the newest patch tag of the same branch in a list of upstream tags, and checks the
// pin age. It never touches the network: the workflow fetches the tags and passes them in.
//
//	go run ./internal/tools/ffmpegfresh -tags tags.txt [-env .config/ffmpeg.env]
//	    [-sources .config/ffmpeg-build/sources.env] [-sources-date YYYY-MM-DD] [-now YYYY-MM-DD]
//	    [-max-age 90] [-body issue.md] [-output github-output.txt]
//
// The tags file has one tag per line (`gh api repos/FFmpeg/FFmpeg/tags --paginate --jq '.[].name'`).
// The result is written as key=value lines (to -output, else stdout): behind, too_old, stale, source,
// pinned, newest, age_days. The exit status is 0 whenever the evaluation worked, whether or not the
// pin is stale; the workflow decides from the keys. It is 2 for a bad input.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

var (
	// n9.0.2-39-g7adfbe40dc-20261031 (BtbN): the patch release the build is based on, then the date.
	btbnVersion = regexp.MustCompile(`^n(\d+)\.(\d+)(?:\.(\d+))?(?:-\d+-g[0-9a-f]+)?-(\d{8})$`)
	// 9.0.2 (the own build of sources.env).
	ownVersion = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?$`)
	// n9.0, n9.0.2: a release tag. n9.0-dev, n9.0.1-rc1 and the like do not match.
	releaseTag = regexp.MustCompile(`^n(\d+)\.(\d+)(?:\.(\d+))?$`)
	assignment = regexp.MustCompile(`^([A-Z0-9_]+)="([^"]*)"\s*(?:#.*)?$`)
)

type release struct {
	major, minor, patch int
}

func (r release) String() string {
	return fmt.Sprintf("n%d.%d.%d", r.major, r.minor, r.patch)
}

func (r release) sameBranch(o release) bool {
	return r.major == o.major && r.minor == o.minor
}

type result struct {
	source   string
	pinned   release
	newest   release
	ageDays  int
	ageKnown bool
	behind   bool
	tooOld   bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ffmpegfresh", flag.ContinueOnError)
	fs.SetOutput(stderr)

	envPath := fs.String("env", ".config/ffmpeg.env", "the pin file")
	sourcesPath := fs.String("sources", ".config/ffmpeg-build/sources.env", "the inputs of the own build")
	sourcesDate := fs.String(
		"sources-date",
		"",
		"date (YYYY-MM-DD) of the last change of the own-build inputs, which is the pin age of the own build",
	)
	tagsPath := fs.String("tags", "", "file with the upstream FFmpeg tags, one per line")
	nowFlag := fs.String("now", "", "today as YYYY-MM-DD (default: the clock)")
	maxAge := fs.Int("max-age", 90, "the oldest a pin may be, in days")
	bodyPath := fs.String("body", "", "write the issue body (Markdown) here when the pin is stale")
	outPath := fs.String("output", "", "write the key=value result here (default: stdout)")

	if err := fs.Parse(argv); err != nil {
		return 2
	}

	if *tagsPath == "" {
		fmt.Fprintln(stderr, "ffmpegfresh: -tags is required")

		return 2
	}

	now := time.Now().UTC()

	if *nowFlag != "" {
		t, err := time.Parse(dateLayout, *nowFlag)
		if err != nil {
			fmt.Fprintf(stderr, "ffmpegfresh: -now: %v\n", err)

			return 2
		}

		now = t
	}

	res, err := evaluate(*envPath, *sourcesPath, *sourcesDate, *tagsPath, now, *maxAge)
	if err != nil {
		fmt.Fprintf(stderr, "ffmpegfresh: %v\n", err)

		return 2
	}

	out := stdout

	if *outPath != "" {
		// The path comes from the command line of the workflow that runs the tool.
		f, err := os.OpenFile(*outPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		if err != nil {
			fmt.Fprintf(stderr, "ffmpegfresh: %v\n", err)

			return 2
		}

		defer f.Close()

		out = f
	}

	fmt.Fprint(out, res.keys())

	if *bodyPath != "" && (res.behind || res.tooOld) {
		if err := os.WriteFile(*bodyPath, []byte(res.body(*maxAge)), 0o600); err != nil {
			fmt.Fprintf(stderr, "ffmpegfresh: %v\n", err)

			return 2
		}
	}

	return 0
}

func evaluate(envPath, sourcesPath, sourcesDate, tagsPath string, now time.Time, maxAge int) (result, error) {
	env, err := readAssignments(envPath)
	if err != nil {
		return result{}, err
	}

	res := result{source: env["FFMPEG_SOURCE"]}

	if res.source == "" {
		res.source = "btbn"
	}

	switch res.source {
	case "btbn":
		m := btbnVersion.FindStringSubmatch(env["FFMPEG_VERSION"])
		if m == nil {
			return result{}, fmt.Errorf(
				"%s: FFMPEG_VERSION %q is not like n9.0.2-39-g7adfbe40dc-20261031",
				envPath,
				env["FFMPEG_VERSION"],
			)
		}

		res.pinned = release{atoi(m[1]), atoi(m[2]), atoi(m[3])}

		built, err := time.Parse("20060102", m[4])
		if err != nil {
			return result{}, fmt.Errorf("%s: the date of FFMPEG_VERSION: %w", envPath, err)
		}

		res.ageDays, res.ageKnown = days(now, built), true
	case "own":
		sources, err := readAssignments(sourcesPath)
		if err != nil {
			return result{}, err
		}

		m := ownVersion.FindStringSubmatch(sources["FFMPEG_BUILD_VERSION"])
		if m == nil {
			return result{}, fmt.Errorf(
				"%s: FFMPEG_BUILD_VERSION %q is not like 9.0.2",
				sourcesPath,
				sources["FFMPEG_BUILD_VERSION"],
			)
		}

		res.pinned = release{atoi(m[1]), atoi(m[2]), atoi(m[3])}

		if sourcesDate != "" {
			built, err := time.Parse(dateLayout, sourcesDate)
			if err != nil {
				return result{}, fmt.Errorf("-sources-date: %w", err)
			}

			res.ageDays, res.ageKnown = days(now, built), true
		}
	default:
		return result{}, fmt.Errorf("%s: FFMPEG_SOURCE must be btbn or own, got %q", envPath, res.source)
	}

	newest, err := newestPatch(tagsPath, res.pinned)
	if err != nil {
		return result{}, err
	}

	res.newest = newest
	res.behind = newest.patch > res.pinned.patch
	res.tooOld = res.ageKnown && res.ageDays > maxAge

	return res, nil
}

// newestPatch is the highest release tag of the pin's branch; the pin itself when the list has none,
// which would mean a list that is empty or of another branch, so it is an error instead.
func newestPatch(path string, pinned release) (release, error) {
	// The path comes from the command line of the workflow that runs the tool.
	f, err := os.Open(path)
	if err != nil {
		return release{}, err
	}

	defer f.Close()

	var (
		newest release
		found  bool
	)

	sc := bufio.NewScanner(f)

	for sc.Scan() {
		m := releaseTag.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}

		r := release{atoi(m[1]), atoi(m[2]), atoi(m[3])}
		if !r.sameBranch(pinned) {
			continue
		}

		if !found || r.patch > newest.patch {
			newest, found = r, true
		}
	}

	if err := sc.Err(); err != nil {
		return release{}, err
	}

	if !found {
		return release{}, fmt.Errorf(
			"%s: no release tag of branch %d.%d (is the list complete?)",
			path,
			pinned.major,
			pinned.minor,
		)
	}

	return newest, nil
}

func readAssignments(path string) (map[string]string, error) {
	// The path comes from the command line of whoever runs the tool.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	out := map[string]string{}

	for line := range strings.SplitSeq(string(data), "\n") {
		if m := assignment.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			out[m[1]] = m[2]
		}
	}

	if len(out) == 0 {
		return nil, errors.New(path + ": no KEY=\"value\" line found")
	}

	return out, nil
}

func days(now, then time.Time) int {
	return int(now.Sub(then).Hours() / 24)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)

	return n
}

func (r result) keys() string {
	age := "unknown"
	if r.ageKnown {
		age = strconv.Itoa(r.ageDays)
	}

	return fmt.Sprintf("behind=%t\ntoo_old=%t\nstale=%t\nsource=%s\npinned=%s\nnewest=%s\nage_days=%s\n",
		r.behind, r.tooOld, r.behind || r.tooOld, r.source, r.pinned, r.newest, age)
}

func (r result) body(maxAge int) string {
	var b strings.Builder

	b.WriteString("The weekly FFmpeg freshness check found the distributed FFmpeg stale (SDD-007 section 12).\n\n")
	fmt.Fprintf(
		&b,
		"- Source in use: `%s`\n- Pinned patch release: `%s`\n- Newest patch release of the branch upstream: `%s`\n",
		r.source,
		r.pinned,
		r.newest,
	)

	if r.ageKnown {
		fmt.Fprintf(&b, "- Pin age: %d days (limit %d)\n", r.ageDays, maxAge)
	}

	b.WriteString("\n")

	if r.behind {
		b.WriteString("A newer patch release exists. Releases built now carry the known flaws of the older one.\n\n")
	}

	if r.tooOld {
		b.WriteString("The pin is older than the limit.\n\n")
	}

	b.WriteString(
		"Bump the pin in `.config/ffmpeg.env` (or the inputs of `.config/ffmpeg-build/sources.env` and a new own-build release), regenerate `internal/ffmpegpins`, and re-run the image smoke test. This issue is updated each week until the pin is fresh.\n",
	)

	return b.String()
}
