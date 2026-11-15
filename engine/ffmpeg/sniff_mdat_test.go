package ffmpeg

import (
	"bytes"
	"testing"
)

// x264Style is the head of an MP4 written without +faststart by an encoder that puts a long text
// SEI first: ftyp, a free box, then an mdat whose first NAL unit is plain ASCII. The first 512
// bytes read as text, and the file is still an ordinary video.
func x264Style() []byte {
	sei := []byte("\x00\x00\x02\x80\x06\x05\xff" + "\xdc\x45\xe9\xbd\xe6\xd9\x48\xb7\x96\x2c\xd8\x20\xd9\x23\xee\xef" +
		"x264 - core 164 r3095 baee400 - H.264/MPEG-4 AVC codec - Copyleft 2003-2022 - " +
		"http://www.videolan.org/x264.html - options: cabac=1 ref=3 deblock=1:0:0 analyse=0x3:0x113 " +
		"me=hex subme=7 psy=1 psy_rd=1.00:0.00 mixed_ref=1 me_range=16 chroma_me=1 trellis=1 8x8dct=1 " +
		"cqm=0 deadzone=21,11 fast_pskip=1 chroma_qp_offset=-2 threads=12 lookahead_threads=2 " +
		"sliced_threads=0 nr=0 decimate=1 interlaced=0 bluray_compat=0 constrained_intra=0 bframes=3 " +
		"b_pyramid=2 b_adapt=1 b_bias=0 direct=1 weightb=1 open_gop=0 weightp=2 keyint=250 " +
		"keyint_min=10 scenecut=40 intra_refresh=0 rc_lookahead=40 rc=crf mbtree=1 crf=23.0 qcomp=0.60 " +
		"qpmin=0 qpmax=69 qpstep=4 ip_ratio=1.40 aq=1:1.00")

	mdat := box("mdat", sei, bytes.Repeat([]byte{0x00, 0x00, 0x01, 0x65, 0x88, 0x84}, 40))

	return bytes.Join([][]byte{
		box("ftyp", []byte("isom"), []byte{0, 0, 2, 0}, []byte("isomiso2avc1mp41")),
		box("free"),
		mdat,
	}, nil)
}

// An MP4 with its moov box at the end is a normal export of ffmpeg and of most encoders: the text
// of its first bytes must not make the sniff refuse it.
func TestSniffAcceptsMP4WithTextAtTheStartOfMdat(t *testing.T) {
	t.Parallel()

	head := x264Style()
	if len(head) < 512 {
		t.Fatalf("the sample is %d bytes, the printable check reads 512", len(head))
	}

	if !mostlyPrintable(head) {
		t.Fatal("the sample does not reproduce the text-like head")
	}

	if err := Sniff(head); err != nil {
		t.Fatalf("an MP4 with a text SEI was refused: %v", err)
	}
}

// The structural check of an ISO base media file is not a way around the refusal of text: a text
// that merely has "ftyp" at byte 4 is still refused.
func TestSniffRefusesTextThatImitatesFtyp(t *testing.T) {
	t.Parallel()

	for name, head := range map[string][]byte{
		"ascii size": append([]byte("abcdftypisom"), bytes.Repeat([]byte("hello world "), 60)...),
		"huge size":  append([]byte("\x7f\xff\xff\xffftypisom"), bytes.Repeat([]byte("hello world "), 60)...),
		"tiny size":  append([]byte("\x00\x00\x00\x04ftypisom"), bytes.Repeat([]byte("hello world "), 60)...),
	} {
		if Sniff(head) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
