package ffmpeg

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg/ffargs"
)

// A request with a Stdin feeds the child's standard input (the encoder of the export); the argv still
// goes through ffargs.Check.
func TestRunFFmpegFeedsStdin(t *testing.T) {
	t.Parallel()
	r, dir := pinnedRunner(t)
	const w, h, frames = 16, 16, 5
	argv, err := ffargs.Cmd{
		Progress: true,
		Inputs:   []ffargs.Input{ffargs.PipeRawVideo("rgba", w, h, ffargs.Rate{Num: 30, Den: 1})},
		Outputs:  []ffargs.Output{{Format: "null", Maps: []string{"0:v:0"}}},
	}.Argv(r.Policy(dir))
	if err != nil {
		t.Fatal(err)
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write(bytes.Repeat([]byte{0x80}, w*h*4*frames))
	}()

	var last Progress
	res, err := r.RunFFmpeg(
		context.Background(),
		Request{Dir: dir, Argv: argv, Deadline: time.Minute, Stdin: pr, OnProgress: func(p Progress) { last = p }},
	)
	if err != nil {
		t.Fatalf("%v\n%s", err, res.Stderr)
	}
	if last.Frame != frames {
		t.Errorf("the encoder saw %d frames, want %d\n%s", last.Frame, frames, res.Stderr)
	}
}

// Cancelling the context ends a run that is waiting for input: no process is left, the error wraps the
// context error.
func TestRunFFmpegStdinCancel(t *testing.T) {
	t.Parallel()
	r, dir := pinnedRunner(t)
	argv, err := ffargs.Cmd{
		Inputs:  []ffargs.Input{ffargs.PipeRawVideo("rgba", 16, 16, ffargs.Rate{Num: 30, Den: 1})},
		Outputs: []ffargs.Output{{Format: "null", Maps: []string{"0:v:0"}}},
	}.Argv(r.Policy(dir))
	if err != nil {
		t.Fatal(err)
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close(); _ = pw.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = r.RunFFmpeg(ctx, Request{Dir: dir, Argv: argv, Deadline: time.Minute, Stdin: pr})
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("err = %v, want the context deadline", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("the cancelled run took %s", time.Since(start))
	}
}
