package ffargs

import (
	"slices"
	"strings"
	"testing"
)

// chaptersCmd is an audio export whose chapters come from meta, its input 1.
func chaptersCmd(meta Input) Cmd {
	return Cmd{
		Inputs: []Input{FileInput(run("mix.wav")), meta},
		Outputs: []Output{{
			Format: "mp4", Path: run("out.mp4"), Maps: []string{"0:a:0"}, NoVideo: true,
			Chapters: 1, Opts: Bitexact(),
		}},
	}
}

func TestMetadataInputArgv(t *testing.T) {
	pol := testPolicy()

	a, err := chaptersCmd(MetadataInput(run("chapters.ffmeta"))).Argv(pol)
	if err != nil {
		t.Fatal(err)
	}

	line := strings.Join(a, " ")
	for _, want := range []string{
		"-protocol_whitelist file,pipe -f ffmetadata -i file:" + run("chapters.ffmeta"),
		"-map_metadata:g -1 -map_metadata:s -1 -map_chapters 1",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("argv lacks %q: %s", want, line)
		}
	}
}

func TestChaptersRefusals(t *testing.T) {
	pol := testPolicy()

	good, err := chaptersCmd(MetadataInput(run("chapters.ffmeta"))).Argv(pol)
	if err != nil {
		t.Fatal(err)
	}

	// -map_chapters 0 names the mix, which is not an ffmetadata input (FF-08).
	bad := slices.Clone(good)
	bad[slices.Index(bad, "-map_chapters")+1] = "0"

	if err := Check(FFmpeg, bad, pol); err == nil || !strings.Contains(err.Error(), "FF-08") {
		t.Errorf("-map_chapters 0 on a media input: %v", err)
	}

	// An ffmetadata file outside the run directory is refused (FF-03).
	if a, err := chaptersCmd(MetadataInput(root() + "/elsewhere/c.ffmeta")).Argv(pol); err == nil {
		t.Errorf("accepted a metadata file outside the run directory: %v", a)
	}

	// Nor is one in the store or in an artifact directory, where another input may come from: the bytes
	// there are a user's file, never the chapters the engine wrote (FF-08, T-89).
	for _, p := range []string{
		root() + "/store/ab/" + digest,
		root() + "/artifacts/" + digest + "/chapters.ffmeta",
	} {
		if a, err := chaptersCmd(MetadataInput(p)).Argv(pol); err == nil {
			t.Errorf("accepted a metadata file outside the run directory: %v", a)
		}
	}

	// -map_chapters 1 with no ffmetadata input at all (FF-08).
	one := Cmd{
		Inputs: []Input{FileInput(run("mix.wav"))},
		Outputs: []Output{{
			Format: "mp4", Path: run("out.mp4"), Maps: []string{"0:a:0"}, NoVideo: true, Chapters: 1, Opts: Bitexact(),
		}},
	}
	if a, err := one.Argv(pol); err == nil {
		t.Errorf("accepted -map_chapters 1 with no metadata input: %v", a)
	}

	// The stream metadata must be dropped too: -map_metadata:g alone is not enough (FF-08).
	i := slices.Index(good, "-map_metadata:s")
	partial := slices.Delete(slices.Clone(good), i, i+2)

	if err := Check(FFmpeg, partial, pol); err == nil || !strings.Contains(err.Error(), "FF-08") {
		t.Errorf("accepted chapters without -map_metadata:s -1: %v", err)
	}

	// -map_metadata:g -1 with -map_chapters -1 is not the plain form either.
	plain := slices.Clone(good)
	plain[slices.Index(plain, "-map_chapters")+1] = "-1"

	if err := Check(FFmpeg, plain, pol); err == nil {
		t.Error("accepted the chapter form of -map_metadata with -map_chapters -1")
	}

	// Extra options on the ffmetadata input are refused (FF-02).
	extra := slices.Insert(slices.Clone(good), slices.Index(good, "ffmetadata")+1, "-sn")
	if err := Check(FFmpeg, extra, pol); err == nil {
		t.Error("accepted -sn on an ffmetadata input")
	}

	// ffmetadata is an input format only (FF-13).
	asOut := slices.Clone(good)
	asOut[slices.Index(asOut, "mp4")] = "ffmetadata"

	if err := Check(FFmpeg, asOut, pol); err == nil {
		t.Error("accepted ffmetadata as an output format")
	}
}

func TestProbeChaptersIsChecked(t *testing.T) {
	a, err := ProbeChapters(testPolicy(), run("out.mp4"))
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(a, "-show_chapters") {
		t.Errorf("argv lacks -show_chapters: %v", a)
	}
}
