package ffargs

import (
	"slices"
	"testing"
)

func TestWebPStillBuilds(t *testing.T) {
	t.Parallel()

	pol := testPolicy()

	lossless, err := WebPStill(pol, run("in.png"), run("out.webp"), 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"libwebp", "-lossless", "image2", "file:" + run("in.png"), "file:" + run("out.webp")} {
		if !slices.Contains(lossless, want) {
			t.Errorf("lossless argv has no %q: %v", want, lossless)
		}
	}

	if slices.Contains(lossless, "-quality") {
		t.Errorf("a lossless argv has no -quality: %v", lossless)
	}

	lossy, err := WebPStill(pol, run("in.png"), run("out.webp"), 75)
	if err != nil {
		t.Fatal(err)
	}

	i := slices.Index(lossy, "-quality")
	if i < 0 || lossy[i+1] != "75" || slices.Contains(lossy, "-lossless") {
		t.Errorf("lossy argv: %v", lossy)
	}

	if err := Check(FFmpeg, lossy, pol); err != nil {
		t.Errorf("the built argv is outside the grammar: %v", err)
	}
}

func TestWebPStillRefuses(t *testing.T) {
	t.Parallel()

	pol := testPolicy()

	for _, q := range []int{-1, 101} {
		if _, err := WebPStill(pol, run("in.png"), run("out.webp"), q); err == nil {
			t.Errorf("quality %d was accepted", q)
		}
	}

	if _, err := WebPStill(pol, run("in.png"), root()+"/elsewhere/out.webp", 80); err == nil {
		t.Error("an output outside the run directory was accepted")
	}

	good, err := WebPStill(pol, run("in.png"), run("out.webp"), 80)
	if err != nil {
		t.Fatal(err)
	}

	for _, extra := range [][]string{
		{"-quality", "0"},
		{"-quality", "101"},
		{"-quality", "80; rm"},
		{"-lossless", "0"},
		{"-c:v", "libwebp_anim"},
		{"-preset", "picture"},
	} {
		if err := Check(FFmpeg, insertBeforeLast(good, extra[0], extra[1]), pol); err == nil {
			t.Errorf("%v was accepted", extra)
		}
	}
}
