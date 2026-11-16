package ingest

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func mp4box(kind string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(body)))

	return append(append(out, kind...), body...)
}

func words(v ...uint32) []byte {
	var b []byte
	for _, w := range v {
		b = binary.BigEndian.AppendUint32(b, w)
	}

	return b
}

// tableOf is a proxy of two VP9 samples whose sample table boxes a test replaces to make them hostile.
func tableOf(stts, stsc, stsz, stco []byte) []byte {
	entry := make([]byte, 78)
	binary.BigEndian.PutUint16(entry[24:], 64)
	binary.BigEndian.PutUint16(entry[26:], 48)

	vpcc := mp4box("vpcC", words(0), []byte{0, 10, 0x80, 1, 1, 1, 0, 0})
	stsd := mp4box("stsd", words(0, 1), mp4box("vp09", entry, vpcc))
	hdlr := mp4box("hdlr", words(0, 0), []byte("vide"), make([]byte, 12))
	mdhd := mp4box("mdhd", words(0, 0, 0, 1000, 2000, 0))
	stbl := mp4box("stbl", stsd, mp4box("stts", stts), mp4box("stsc", stsc), mp4box("stsz", stsz), mp4box("stco", stco))
	trak := mp4box("trak", mp4box("mdia", hdlr, mdhd, mp4box("minf", stbl)))

	return append(mp4box("ftyp", []byte("isom"), words(0x200)), mp4box("moov", trak)...)
}

func goodTables() (stts, stsc, stsz, stco []byte) {
	return words(0, 1, 2, 1000), words(0, 1, 1, 2, 1), words(0, 0, 2, 100, 200), words(0, 1, 4096)
}

// The sample table of a proxy is read by the product from a file it wrote itself, but a project can carry
// a proxy that came from anywhere: every inconsistent table is an error and none of them can panic or
// allocate what its counts claim (SDD-011 4.3.6, SDD-007 9).
func TestChunkIndexRefusesHostileSampleTables(t *testing.T) {
	t.Parallel()

	stts, stsc, stsz, stco := goodTables()

	base := tableOf(stts, stsc, stsz, stco)

	idx, err := readChunkIndex(bytes.NewReader(base), int64(len(base)))
	if err != nil || len(idx.Sizes) != 2 || idx.Offsets[1] != 4196 || idx.Width != 64 {
		t.Fatalf("the base table: %+v, %v", idx, err)
	}

	cases := map[string][4][]byte{
		"two billion samples":            {stts, stsc, words(0, 0, 0x7fffffff), stco},
		"sizes shorter than the count":   {stts, stsc, words(0, 0, 5, 100), stco},
		"more chunks than offsets":       {stts, stsc, stsz, words(0, 0xffffffff, 4096)},
		"more runs than entries":         {stts, words(0, 0xffffffff, 1, 2, 1), stsz, stco},
		"a run in a chunk that is none":  {stts, words(0, 1, 9, 2, 1), stsz, stco},
		"a negative-looking run":         {stts, words(0, 1, 1, 0xffffffff, 1), stsz, stco},
		"more samples than sizes":        {stts, words(0, 1, 1, 3, 1), stsz, stco},
		"fewer samples than sizes":       {stts, words(0, 1, 1, 1, 1), stsz, stco},
		"times for four billion samples": {words(0, 1, 0xffffffff, 1000), stsc, stsz, stco},
		"times shorter than the count":   {words(0, 9, 2, 1000), stsc, stsz, stco},
		"no times":                       {words(0, 0), stsc, stsz, stco},
		"truncated tables":               {stts[:6], stsc[:5], stsz[:9], stco[:3]},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file := tableOf(c[0], c[1], c[2], c[3])
			if _, err := readChunkIndex(bytes.NewReader(file), int64(len(file))); err == nil {
				t.Fatal("a hostile sample table was read")
			}
		})
	}
}

func FuzzChunkIndex(f *testing.F) {
	stts, stsc, stsz, stco := goodTables()
	f.Add(tableOf(stts, stsc, stsz, stco))
	f.Add(tableOf(stts, words(0, 1, 9, 2, 1), stsz, stco))
	f.Add(tableOf(words(0, 1, 0xffffffff, 1000), stsc, stsz, stco))
	f.Add([]byte("\x00\x00\x00\x01moov\xff\xff\xff\xff\xff\xff\xff\xff"))

	f.Fuzz(func(_ *testing.T, file []byte) {
		// Only the absence of a panic and of an unbounded allocation is asserted.
		_, _ = readChunkIndex(bytes.NewReader(file), int64(len(file)))
	})
}
