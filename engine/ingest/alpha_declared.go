package ingest

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// declaresAlpha reports whether the file itself declares an alpha channel (SDD-011 section 4.3.6). The pixel format
// of the probe says so for every accepted input but two: FFmpeg decodes every GIF to bgra and every lossless WebP to
// argb, with or without transparency, so an opaque GIF or WebP would read as "declares alpha and has none". A GIF
// declares it with the transparent colour flag of a graphic control extension, a WebP with the alpha flag of its
// VP8X header or the alpha hint of a lossless VP8L picture. Any other file declares it by its pixel format, which the
// plan already read. A GIF or a WebP whose structure cannot be read is not called declared: no finding is invented.
func declaresAlpha(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReader(f)

	head, err := r.Peek(12)
	if err != nil {
		return true
	}

	switch {
	case bytes.HasPrefix(head, []byte("GIF8")):
		return gifDeclaresAlpha(r)
	case bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return webpDeclaresAlpha(r)
	}

	return true
}

// gifDeclaresAlpha walks the blocks of a GIF (header, screen descriptor, colour tables, extensions and image data) to
// the first graphic control extension whose transparent colour flag is set.
func gifDeclaresAlpha(r *bufio.Reader) bool {
	// "GIF89a", the width, the height and the packed byte of the logical screen descriptor.
	screen := make([]byte, 13)
	if _, err := io.ReadFull(r, screen); err != nil || !skipColorTable(r, screen[10]) {
		return false
	}

	for {
		b, err := r.ReadByte()
		if err != nil {
			return false
		}

		switch b {
		case 0x21: // an extension: its label, then its sub-blocks
			label, err := r.ReadByte()
			if err != nil {
				return false
			}

			if label == 0xF9 {
				// The graphic control extension: a sub-block of 4 bytes whose first is packed; bit 0 is the
				// transparent colour flag.
				gce, err := r.Peek(2)
				if err != nil {
					return false
				}

				if gce[1]&1 != 0 {
					return true
				}
			}
		case 0x2C: // an image: its descriptor, its local colour table, the LZW code size, then its sub-blocks
			desc := make([]byte, 9) // left, top, width, height and the packed byte
			if _, err := io.ReadFull(r, desc); err != nil || !skipColorTable(r, desc[8]) {
				return false
			}

			if _, err := r.Discard(1); err != nil {
				return false
			}
		default: // the trailer (0x3B) or a block the format does not have
			return false
		}

		if !skipSubBlocks(r) {
			return false
		}
	}
}

// skipColorTable skips the colour table that a packed byte of a GIF descriptor announces, if any.
func skipColorTable(r *bufio.Reader, packed byte) bool {
	if packed&0x80 == 0 {
		return true
	}

	_, err := r.Discard(3 << (int(packed&7) + 1))

	return err == nil
}

// skipSubBlocks skips a chain of GIF sub-blocks up to its empty terminator.
func skipSubBlocks(r *bufio.Reader) bool {
	for {
		n, err := r.ReadByte()
		if err != nil {
			return false
		}

		if n == 0 {
			return true
		}

		if _, err := r.Discard(int(n)); err != nil {
			return false
		}
	}
}

// webpDeclaresAlpha reads the first chunk of a WebP: VP8X carries the alpha flag of the whole file (an animation
// included), VP8L the alpha hint of a lossless picture, and a lossy VP8 picture has no alpha.
func webpDeclaresAlpha(r *bufio.Reader) bool {
	chunk := make([]byte, 12+8+5) // RIFF header, chunk header, and the first bytes of its payload
	if _, err := io.ReadFull(r, chunk[:20]); err != nil {
		return false
	}

	payload := chunk[20:]

	switch string(chunk[12:16]) {
	case "VP8X":
		if _, err := io.ReadFull(r, payload[:1]); err != nil {
			return false
		}

		return payload[0]&0x10 != 0
	case "VP8L":
		// The signature 0x2F, then 14 bits of width, 14 of height and the alpha_is_used bit, least significant first.
		if _, err := io.ReadFull(r, payload); err != nil || payload[0] != 0x2F {
			return false
		}

		return binary.LittleEndian.Uint32(payload[1:5])>>28&1 == 1
	}

	return false
}

// rawAlphaOpaque reports whether every pixel of a file of raw RGBA frames is fully opaque: the animated image is
// sampled on every frame, not only on the first one that its normalized picture shows.
func rawAlphaOpaque(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 1<<16) // a multiple of 4, so every read starts on a pixel
	for {
		n, err := io.ReadFull(f, buf)
		for i := 3; i < n; i += 4 {
			if buf[i] != 0xFF {
				return false
			}
		}

		if err != nil {
			return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
		}
	}
}
