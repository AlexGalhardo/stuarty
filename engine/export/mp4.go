package export

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// boxes is what the verification reads from the top level of an MP4: where moov and mdat start and
// whether the boxes fill the file exactly. A truncated file has an mdat that claims more than the
// file holds, which ffprobe does not report when moov (faststart) is at the front.
type boxes struct {
	moov, mdat int64 // offset of the first box of that type, -1 when absent
	complete   bool
}

// boxAt reads the header of the box at pos: its size (the rest of the file for the last box) and its
// type. ok is false when the header does not fit in the file or states a size it cannot hold.
func boxAt(f io.ReaderAt, pos, fileSize int64) (n int64, kind string, ok bool, err error) {
	var hdr [16]byte
	if fileSize-pos < 8 {
		return 0, "", false, nil
	}
	if _, err := f.ReadAt(hdr[:8], pos); err != nil && !errors.Is(err, io.EOF) {
		return 0, "", false, err
	}
	n, kind = int64(binary.BigEndian.Uint32(hdr[0:4])), string(hdr[4:8])
	switch {
	case n == 0:
		return fileSize - pos, kind, true, nil
	case n == 1:
		if fileSize-pos < 16 {
			return 0, kind, false, nil
		}
		if _, err := f.ReadAt(hdr[8:16], pos+8); err != nil {
			return 0, "", false, err
		}
		big := binary.BigEndian.Uint64(hdr[8:16])
		if big < 16 || big > math.MaxInt64 {
			return 0, kind, false, nil
		}
		size := int64(big)
		return size, kind, true, nil
	case n < 8:
		return 0, "", false, fmt.Errorf("a box of %d bytes at offset %d", n, pos)
	}
	return n, kind, true, nil
}

// readBoxes walks the top-level boxes of an MP4 file.
func readBoxes(path string) (boxes, error) {
	f, err := os.Open(path)
	if err != nil {
		return boxes{}, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return boxes{}, err
	}
	size := st.Size()

	out := boxes{moov: -1, mdat: -1, complete: true}
	for pos := int64(0); pos < size; {
		n, kind, ok, err := boxAt(f, pos, size)
		if err != nil {
			return boxes{}, err
		}
		if !ok || pos+n > size {
			out.complete = false
			if ok && kind == "mdat" && out.mdat < 0 {
				out.mdat = pos
			}
			return out, nil
		}
		switch {
		case kind == "moov" && out.moov < 0:
			out.moov = pos
		case kind == "mdat" && out.mdat < 0:
			out.mdat = pos
		}
		pos += n
	}
	return out, nil
}
