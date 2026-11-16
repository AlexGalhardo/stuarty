package ingest

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// ChunkIndexVersion is the version field of chunks-1.json (SDD-011 section 4.3.6).
const ChunkIndexVersion = 1

// ChunkIndex is what a WebCodecs decoder needs to read the video of a proxy without a demuxer: the
// codec string and the coded size for VideoDecoder.configure, and, for every sample in decode order,
// where its bytes are in proxy-1.mp4, how many there are, its presentation time and whether it is a
// key frame. The arrays have one entry per sample.
type ChunkIndex struct {
	Version int    `json:"version"`
	Codec   string `json:"codec"`
	Width   int    `json:"coded_width"`
	Height  int    `json:"coded_height"`
	// DurationUS is the duration of the video track in microseconds.
	DurationUS int64 `json:"duration_us"`
	// Offsets and Sizes are the byte range of each sample in the proxy file.
	Offsets []int64 `json:"offsets"`
	Sizes   []int64 `json:"sizes"`
	// TimestampsUS is the presentation time of each sample in microseconds, as EncodedVideoChunk
	// takes it; the first sample of the proxy is at 0 or later.
	TimestampsUS []int64 `json:"timestamps_us"`
	// Keyframes lists the indexes of the samples that can start a decode.
	Keyframes []int `json:"keyframes"`
}

// ErrNoChunkIndex means the file has no video track the index can describe (an audio proxy, or a
// codec other than VP9, which is what the proxy writer produces).
var ErrNoChunkIndex = errors.New("ingest: the proxy has no VP9 video track to index")

const (
	maxMetaBox   = 256 << 20 // a sample table box larger than this is not read
	boxHeaderLen = 8
)

type mp4Box struct {
	typ        string
	start, end int64 // the whole box, header included
	body       int64 // where the payload starts
}

// boxes lists the boxes between from and to.
func boxes(r io.ReaderAt, from, to int64) ([]mp4Box, error) {
	var out []mp4Box

	for pos := from; pos+boxHeaderLen <= to; {
		var h [16]byte
		if _, err := r.ReadAt(h[:boxHeaderLen], pos); err != nil {
			return nil, err
		}

		size, hdr := int64(binary.BigEndian.Uint32(h[:4])), int64(boxHeaderLen)

		switch size {
		case 0:
			size = to - pos
		case 1:
			if _, err := r.ReadAt(h[8:16], pos+8); err != nil {
				return nil, err
			}

			size, hdr = asInt64(binary.BigEndian.Uint64(h[8:16])), 16
			if size < 0 || size > to-pos {
				return nil, errors.New("a box is larger than its parent")
			}
		}

		if size < hdr || pos+size > to {
			return nil, errors.New("a box does not fit in its parent")
		}

		out = append(out, mp4Box{typ: string(h[4:8]), start: pos, end: pos + size, body: pos + hdr})
		pos += size
	}

	return out, nil
}

func find(bs []mp4Box, typ string) (mp4Box, bool) {
	for _, b := range bs {
		if b.typ == typ {
			return b, true
		}
	}

	return mp4Box{}, false
}

func children(r io.ReaderAt, b mp4Box) ([]mp4Box, error) { return boxes(r, b.body, b.end) }

// payload reads the body of a box.
func payload(r io.ReaderAt, b mp4Box) ([]byte, error) {
	n := b.end - b.body
	if n > maxMetaBox {
		return nil, errors.New("a sample table box is too large")
	}

	buf := make([]byte, n)
	_, err := r.ReadAt(buf, b.body)

	return buf, err
}

// descend follows the box types in path from b and returns the box at the end.
func descend(r io.ReaderAt, b mp4Box, path ...string) (mp4Box, error) {
	for _, typ := range path {
		cs, err := children(r, b)
		if err != nil {
			return mp4Box{}, err
		}

		next, ok := find(cs, typ)
		if !ok {
			return mp4Box{}, fmt.Errorf("no %s box", typ)
		}

		b = next
	}

	return b, nil
}

type sampleTable struct {
	stts, ctts, stss, stsz, stsc, stco []byte
	co64                               bool
	hasCtts, hasStss                   bool
}

// ReadChunkIndex reads the sample table of the first video track of an MP4 file, in Go, from the
// boxes of the file alone (no FFmpeg). It returns ErrNoChunkIndex when the file has no VP9 video.
func ReadChunkIndex(path string) (*ChunkIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}

	return readChunkIndex(f, st.Size())
}

func readChunkIndex(r io.ReaderAt, size int64) (*ChunkIndex, error) {
	top, err := boxes(r, 0, size)
	if err != nil {
		return nil, err
	}

	moov, ok := find(top, "moov")
	if !ok {
		return nil, errors.New("the file has no moov box")
	}

	traks, err := children(r, moov)
	if err != nil {
		return nil, err
	}

	for _, t := range traks {
		if t.typ != "trak" {
			continue
		}

		idx, err := indexOfTrack(r, t)
		if errors.Is(err, ErrNoChunkIndex) {
			continue
		}

		return idx, err
	}

	return nil, ErrNoChunkIndex
}

func indexOfTrack(r io.ReaderAt, trak mp4Box) (*ChunkIndex, error) {
	mdia, err := descend(r, trak, "mdia")
	if err != nil {
		return nil, err
	}

	hdlr, err := descend(r, mdia, "hdlr")
	if err != nil {
		return nil, err
	}

	hb, err := payload(r, hdlr)
	if err != nil || len(hb) < 12 || string(hb[8:12]) != "vide" {
		return nil, ErrNoChunkIndex
	}

	mdhd, err := descend(r, mdia, "mdhd")
	if err != nil {
		return nil, err
	}

	mb, err := payload(r, mdhd)
	if err != nil {
		return nil, err
	}

	timescale, mediaDur, err := parseMdhd(mb)
	if err != nil {
		return nil, err
	}

	stbl, err := descend(r, mdia, "minf", "stbl")
	if err != nil {
		return nil, err
	}

	cs, err := children(r, stbl)
	if err != nil {
		return nil, err
	}

	idx := &ChunkIndex{Version: ChunkIndexVersion}

	stsd, ok := find(cs, "stsd")
	if !ok {
		return nil, errors.New("no stsd box")
	}

	if err := parseStsd(r, stsd, idx); err != nil {
		return nil, err
	}

	var tab sampleTable

	for _, c := range cs {
		var dst *[]byte

		switch c.typ {
		case "stts":
			dst = &tab.stts
		case "ctts":
			dst, tab.hasCtts = &tab.ctts, true
		case "stss":
			dst, tab.hasStss = &tab.stss, true
		case "stsz":
			dst = &tab.stsz
		case "stsc":
			dst = &tab.stsc
		case "stco":
			dst = &tab.stco
		case "co64":
			dst, tab.co64 = &tab.stco, true
		default:
			continue
		}

		if *dst, err = payload(r, c); err != nil {
			return nil, err
		}
	}

	shift, err := editShift(r, trak)
	if err != nil {
		return nil, err
	}

	if err := tab.fill(idx, timescale, shift); err != nil {
		return nil, err
	}

	idx.DurationUS = scaleUS(mediaDur-shift, timescale)

	return idx, nil
}

func parseMdhd(b []byte) (timescale uint32, duration int64, err error) {
	if len(b) < 24 {
		return 0, 0, errors.New("short mdhd")
	}

	if b[0] == 1 {
		if len(b) < 36 {
			return 0, 0, errors.New("short mdhd")
		}

		return binary.BigEndian.Uint32(b[20:24]), asInt64(binary.BigEndian.Uint64(b[24:32])), nil
	}

	return binary.BigEndian.Uint32(b[12:16]), int64(binary.BigEndian.Uint32(b[16:20])), nil
}

// editShift is the media time of the first edit of the track, which presentation times are counted
// from; zero when there is no edit list.
func editShift(r io.ReaderAt, trak mp4Box) (int64, error) {
	elst, err := descend(r, trak, "edts", "elst")
	if err != nil {
		return 0, nil //nolint:nilerr // no edit list is the normal case
	}

	b, err := payload(r, elst)
	if err != nil || len(b) < 8 {
		return 0, err
	}

	n := binary.BigEndian.Uint32(b[4:8])
	if n == 0 {
		return 0, nil
	}

	var t int64

	switch {
	case b[0] == 1 && len(b) >= 8+20:
		t = asInt64(binary.BigEndian.Uint64(b[16:24]))
	case b[0] == 0 && len(b) >= 8+12:
		t = signed32(binary.BigEndian.Uint32(b[12:16]))
	default:
		return 0, errors.New("short elst")
	}

	return max(t, 0), nil
}

// parseStsd reads the codec of the first sample entry: only VP9 (vp09 with its vpcC box).
func parseStsd(r io.ReaderAt, stsd mp4Box, idx *ChunkIndex) error {
	const entryHeader = 8 + 78 // box header and the fixed fields of a visual sample entry

	es, err := boxes(r, stsd.body+8, stsd.end)
	if err != nil || len(es) == 0 {
		return errors.New("no sample entry")
	}

	e := es[0]
	if e.typ != "vp09" {
		return ErrNoChunkIndex
	}

	if e.end-e.start < entryHeader {
		return errors.New("short sample entry")
	}

	var wh [4]byte
	if _, err := r.ReadAt(wh[:], e.start+32); err != nil {
		return err
	}

	idx.Width, idx.Height = int(binary.BigEndian.Uint16(wh[:2])), int(binary.BigEndian.Uint16(wh[2:]))

	cs, err := boxes(r, e.start+entryHeader, e.end)
	if err != nil {
		return err
	}

	vpcc, ok := find(cs, "vpcC")
	if !ok {
		return errors.New("no vpcC box")
	}

	b, err := payload(r, vpcc)
	if err != nil || len(b) < 12 {
		return errors.New("short vpcC box")
	}

	// version and flags (4), profile, level, bit depth / chroma / full range, primaries, transfer, matrix.
	profile, level, packed := b[4], b[5], b[6]
	idx.Codec = fmt.Sprintf("vp09.%02d.%02d.%02d.%02d.%02d.%02d.%02d.%02d",
		profile, level, packed>>4, packed>>1&7, b[7], b[8], b[9], packed&1)

	return nil
}

func scaleUS(v int64, timescale uint32) int64 {
	if timescale == 0 {
		return 0
	}

	return (v*1_000_000 + int64(timescale)/2) / int64(timescale)
}

// asInt64 and signed32 reinterpret the bits of a field that the format defines as signed or that a
// caller checks for a negative value right after; no value is truncated.
func asInt64(v uint64) int64 { return int64(v) } //nolint:gosec // the callers refuse a negative result

func signed32(v uint32) int64 { return int64(int32(v)) } //nolint:gosec // ctts and elst times are signed 32 bit fields

// u32 reads the nth big endian word of b.
func u32(b []byte, n int) uint32 { return binary.BigEndian.Uint32(b[4*n:]) }

// fill expands the sample table into the per-sample arrays of idx.
func (t *sampleTable) fill(idx *ChunkIndex, timescale uint32, shift int64) error {
	if len(t.stsz) < 12 || len(t.stts) < 8 || len(t.stsc) < 8 || len(t.stco) < 8 {
		return errors.New("an incomplete sample table")
	}

	count := int64(u32(t.stsz, 2))
	constant := int64(u32(t.stsz, 1))

	if constant == 0 && int64(len(t.stsz)) < 12+4*count {
		return errors.New("stsz is shorter than its sample count")
	}

	if count > maxMetaBox/4 {
		return errors.New("too many samples")
	}

	idx.Sizes = make([]int64, count)
	idx.Offsets = make([]int64, count)
	idx.TimestampsUS = make([]int64, count)

	for i := range idx.Sizes {
		if constant != 0 {
			idx.Sizes[i] = constant
		} else {
			idx.Sizes[i] = int64(u32(t.stsz, 3+i))
		}
	}

	if err := t.offsets(idx); err != nil {
		return err
	}

	if err := t.times(idx, timescale, shift); err != nil {
		return err
	}

	idx.Keyframes = []int{}

	if !t.hasStss {
		for i := range idx.Sizes {
			idx.Keyframes = append(idx.Keyframes, i)
		}

		return nil
	}

	if len(t.stss) < 8 {
		return errors.New("short stss")
	}

	n := int(u32(t.stss, 1))
	if len(t.stss) < 8+4*n {
		return errors.New("stss is shorter than its count")
	}

	for i := range n {
		s := int(u32(t.stss, 2+i))
		if s < 1 || int64(s) > count {
			return errors.New("stss names a sample that does not exist")
		}

		idx.Keyframes = append(idx.Keyframes, s-1)
	}

	return nil
}

// offsets places every sample in the file from the chunk offsets and the samples-per-chunk runs.
func (t *sampleTable) offsets(idx *ChunkIndex) error {
	word := 4
	if t.co64 {
		word = 8
	}

	chunks := int(u32(t.stco, 1))
	if len(t.stco) < 8+word*chunks {
		return errors.New("the chunk offsets are shorter than their count")
	}

	runs := int(u32(t.stsc, 1))
	if len(t.stsc) < 8+12*runs {
		return errors.New("stsc is shorter than its count")
	}

	chunkOffset := func(c int) int64 {
		if t.co64 {
			return asInt64(binary.BigEndian.Uint64(t.stco[8+8*c:]))
		}

		return int64(u32(t.stco, 2+c))
	}

	sample := 0

	for r := range runs {
		first := int(u32(t.stsc, 2+3*r))
		perChunk := int(u32(t.stsc, 3+3*r))
		last := chunks

		if r+1 < runs {
			last = int(u32(t.stsc, 2+3*(r+1))) - 1
		}

		if first < 1 || perChunk < 0 || last > chunks {
			return errors.New("stsc names a chunk that does not exist")
		}

		for c := first - 1; c < last; c++ {
			off := chunkOffset(c)

			for range perChunk {
				if sample >= len(idx.Sizes) {
					return errors.New("stsc places more samples than stsz has")
				}

				idx.Offsets[sample] = off
				off += idx.Sizes[sample]
				sample++
			}
		}
	}

	if sample != len(idx.Sizes) {
		return errors.New("stsc places fewer samples than stsz has")
	}

	return nil
}

// times sets the presentation time of every sample: decode time from stts, plus the composition
// offset of ctts, minus the start of the first edit.
func (t *sampleTable) times(idx *ChunkIndex, timescale uint32, shift int64) error {
	n := int(u32(t.stts, 1))
	if len(t.stts) < 8+8*n {
		return errors.New("stts is shorter than its count")
	}

	dts := make([]int64, 0, len(idx.Sizes))

	var at int64

	for e := range n {
		cnt, delta := int64(u32(t.stts, 2+2*e)), int64(u32(t.stts, 3+2*e))
		if int64(len(dts))+cnt > int64(len(idx.Sizes)) {
			return errors.New("stts covers more samples than stsz has")
		}

		for range cnt {
			dts = append(dts, at)
			at += delta
		}
	}

	if len(dts) != len(idx.Sizes) {
		return errors.New("stts covers fewer samples than stsz has")
	}

	comp := make([]int64, len(dts))

	if t.hasCtts {
		if len(t.ctts) < 8 {
			return errors.New("short ctts")
		}

		cn, i := int(u32(t.ctts, 1)), 0
		if len(t.ctts) < 8+8*cn {
			return errors.New("ctts is shorter than its count")
		}

		for e := range cn {
			cnt, off := int(u32(t.ctts, 2+2*e)), signed32(u32(t.ctts, 3+2*e))
			for range cnt {
				if i >= len(comp) {
					return errors.New("ctts covers more samples than stsz has")
				}

				comp[i] = off
				i++
			}
		}
	}

	for i := range dts {
		idx.TimestampsUS[i] = scaleUS(dts[i]+comp[i]-shift, timescale)
	}

	return nil
}

// writeChunks writes chunks.json next to the video proxy, read from the proxy's own sample table, so
// the browser can decode the proxy with WebCodecs without a demuxer (ADR-0029 item 5).
func (j *job) writeChunks() error {
	idx, err := ReadChunkIndex(j.path("proxy.mp4"))
	if err != nil {
		return ingestErr(
			ReasonDecodeFailed,
			"The chunk index of the proxy could not be written.",
			map[string]any{"step": StepMux},
			err,
		)
	}

	if err := j.writeJSON("chunks.json", idx); err != nil {
		return ingestErr(
			ReasonDecodeFailed,
			"The chunk index of the proxy could not be written.",
			map[string]any{"step": StepMux},
			err,
		)
	}

	j.add(workFile{kind: "chunks", profile: ProfileChunks, ext: "json", name: "chunks.json"})

	return nil
}
