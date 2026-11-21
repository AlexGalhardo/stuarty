package export

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"hash/adler32"
	"hash/crc32"
	"image"
	"image/color"
	"runtime"
	"sync"
)

// pngSegmentRows is the rows each independently compressed piece of the PNG holds. It is part of the format
// this encoder writes, never a function of the machine: the same pixels give the same bytes whatever the CPU
// count (NFR-062).
const pngSegmentRows = 128

// encodePNGParallel writes the picture as a PNG, compressing pieces of it on every CPU. A 4,000 by 4,000 picture
// takes half a second on one core with the standard encoder at its fastest level, which is a quarter of the
// budget of an export (NFR-061); the pieces are rows of the image filtered with Sub (which reads only the pixel
// to the left, so a row needs no other) and deflated on their own, each ended on a byte boundary so that the
// stream is one valid zlib stream. The alpha is straight, as the standard encoder writes it, and a picture with
// no transparency is written as RGB. The file is lossless and the same on every machine, and it is not the
// file of the standard encoder.
func encodePNGParallel(img *image.RGBA) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	opaque := true

	for y := 0; y < h && opaque; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 255 {
				opaque = false

				break
			}
		}
	}

	bpp, colourType := 4, byte(6)
	if opaque {
		bpp, colourType = 3, 2
	}

	segments := (h + pngSegmentRows - 1) / pngSegmentRows
	parts := make([]pngPart, segments)
	jobs := make(chan int)

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed error
	)

	for range min(runtime.GOMAXPROCS(0), segments) {
		wg.Go(func() {
			for s := range jobs {
				p, err := compressSegment(img, w, h, s, bpp, opaque, s == segments-1)
				if err != nil {
					mu.Lock()
					failed = err
					mu.Unlock()
				}

				parts[s] = p
			}
		})
	}

	for s := range segments {
		jobs <- s
	}

	close(jobs)
	wg.Wait()

	if failed != nil {
		return nil, failed
	}

	var data bytes.Buffer

	data.Write([]byte{0x78, 0x01}) // zlib header: deflate, 32 KiB window, fastest

	sum := uint32(1)

	for s := range parts {
		data.Write(parts[s].deflated)

		if s == 0 {
			sum = parts[s].adler
		} else {
			sum = adlerCombine(sum, parts[s].adler, parts[s].rawLen)
		}
	}

	_ = binary.Write(&data, binary.BigEndian, sum)

	var out bytes.Buffer

	out.Grow(data.Len() + 64)
	out.WriteString("\x89PNG\r\n\x1a\n")

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w)) //nolint:gosec // G115: bounded by the export limits
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h)) //nolint:gosec // G115: bounded by the export limits
	ihdr[8], ihdr[9] = 8, colourType
	writePNGChunk(&out, "IHDR", ihdr)
	writePNGChunk(&out, "IDAT", data.Bytes())
	writePNGChunk(&out, "IEND", nil)

	return out.Bytes(), nil
}

type pngPart struct {
	deflated []byte
	adler    uint32
	rawLen   int64
}

// compressSegment filters and deflates the rows of one segment. Every segment but the last ends with a sync
// flush, which leaves the deflate stream on a byte boundary with no final block; the last one is closed.
func compressSegment(img *image.RGBA, w, h, s, bpp int, opaque, last bool) (pngPart, error) {
	y0, y1 := s*pngSegmentRows, min((s+1)*pngSegmentRows, h)
	rowLen := 1 + w*bpp
	raw := make([]byte, (y1-y0)*rowLen)
	straight := make([]byte, w*bpp)

	for y := y0; y < y1; y++ {
		src := img.Pix[y*img.Stride : y*img.Stride+w*4]

		if opaque {
			for x := range w {
				copy(straight[x*3:x*3+3], src[x*4:x*4+3])
			}
		} else {
			for x := range w {
				p := src[x*4 : x*4+4]

				switch p[3] {
				case 255:
					copy(straight[x*4:x*4+4], p)
				case 0:
					clear(straight[x*4 : x*4+4])
				default:
					c, _ := color.NRGBAModel.Convert(color.RGBA{R: p[0], G: p[1], B: p[2], A: p[3]}).(color.NRGBA)
					straight[x*4], straight[x*4+1], straight[x*4+2], straight[x*4+3] = c.R, c.G, c.B, c.A
				}
			}
		}

		out := raw[(y-y0)*rowLen : (y-y0+1)*rowLen]
		out[0] = 1 // the Sub filter

		for i, v := range straight {
			if i < bpp {
				out[1+i] = v

				continue
			}

			out[1+i] = v - straight[i-bpp]
		}
	}

	var buf bytes.Buffer

	fw, err := flate.NewWriter(&buf, flate.BestSpeed)
	if err != nil {
		return pngPart{}, err
	}

	if _, err := fw.Write(raw); err != nil {
		return pngPart{}, err
	}

	if last {
		err = fw.Close()
	} else {
		err = fw.Flush()
	}

	if err != nil {
		return pngPart{}, err
	}

	return pngPart{deflated: buf.Bytes(), adler: adler32.Checksum(raw), rawLen: int64(len(raw))}, nil
}

func writePNGChunk(out *bytes.Buffer, kind string, data []byte) {
	var n [4]byte

	binary.BigEndian.PutUint32(n[:], uint32(len(data))) //nolint:gosec // G115: a chunk is far below 4 GiB
	out.Write(n[:])

	start := out.Len()

	out.WriteString(kind)
	out.Write(data)

	binary.BigEndian.PutUint32(n[:], crc32.ChecksumIEEE(out.Bytes()[start:]))
	out.Write(n[:])
}

// adlerCombine is the Adler-32 of two blocks from their checksums and the length of the second, as zlib's
// adler32_combine.
func adlerCombine(a1, a2 uint32, len2 int64) uint32 {
	const base = 65521

	rem := uint32(len2 % base) //nolint:gosec // G115: below 65521
	sum1 := a1 & 0xffff
	sum2 := (rem * sum1) % base
	sum1 += (a2 & 0xffff) + base - 1
	sum2 += (a1 >> 16) + (a2 >> 16) + base - rem

	if sum1 >= base {
		sum1 -= base
	}

	if sum1 >= base {
		sum1 -= base
	}

	if sum2 >= base<<1 {
		sum2 -= base << 1
	}

	if sum2 >= base {
		sum2 -= base
	}

	return sum1 | sum2<<16
}
