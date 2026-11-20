// Command installericon scales the official Stuarty icon (a square PNG) into every copy the
// project ships: the favicon and the logo of the site and of the web UI, the icon of the
// desktop window and tray, and the icon of the installers. The outputs are committed; run it
// again only when the icon changes:
//
//	go run ./internal/tools/installericon .config/installers/assets/stuarty-icon.png \
//		.config/installers/assets/stuarty.png=256 .config/installers/assets/stuarty.ico \
//		desktop/shell/app/tray.png=256 desktop/shell/app/tray.ico \
//		site/public/logo.png=256 site/public/icon.png=64 web/resources/public/icon.png=64
//
// An output ending in .ico packs the sizes Windows asks for (16 to 256) into one file.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

func scale(src image.Image, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)

	return dst
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer

	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ico packs PNG images into one .ico (PNG entries are valid since Windows Vista).
func ico(src image.Image, sizes []int) ([]byte, error) {
	imgs := make([][]byte, 0, len(sizes))

	for _, s := range sizes {
		b, err := encodePNG(scale(src, s))
		if err != nil {
			return nil, err
		}

		imgs = append(imgs, b)
	}

	var out bytes.Buffer

	le := func(v any) { _ = binary.Write(&out, binary.LittleEndian, v) }
	le(uint16(0))
	le(uint16(1))
	le(uint16(len(sizes) & 0xFFFF))

	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := byte(s & 0xFF)
		if s >= 256 {
			w = 0
		}

		out.Write([]byte{w, w, 0, 0})
		le(uint16(1))
		le(uint16(32))
		le(uint32(len(imgs[i]) & math.MaxUint32))
		le(uint32(offset & math.MaxUint32))
		offset += len(imgs[i])
	}

	for _, b := range imgs {
		out.Write(b)
	}

	return out.Bytes(), nil
}

// output turns one argument, "path=size" or "path.ico", into the bytes of that file.
func output(src image.Image, arg string) (string, []byte, error) {
	if strings.HasSuffix(strings.ToLower(arg), ".ico") {
		b, err := ico(src, icoSizes)

		return arg, b, err
	}

	path, sizeText, ok := strings.Cut(arg, "=")
	if !ok {
		return "", nil, fmt.Errorf("%q: want path=size or a .ico path", arg)
	}

	size, err := strconv.Atoi(sizeText)
	if err != nil || size < 16 || size > 1024 {
		return "", nil, fmt.Errorf("%q: the size must be a number from 16 to 1024", arg)
	}

	b, err := encodePNG(scale(src, size))

	return path, b, err
}

func run(source string, outputs []string) error {
	f, err := os.Open(source) //nolint:gosec // G304: the source icon is the argument of a developer tool
	if err != nil {
		return err
	}
	defer f.Close()

	src, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}

	if b := src.Bounds(); b.Dx() != b.Dy() {
		return errors.New("the source icon must be square")
	}

	for _, arg := range outputs {
		path, data, err := output(src, arg)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:gosec // G703: a developer tool
			return err
		}

		if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // G703: a developer tool
			return err
		}
	}

	return nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: installericon <source.png> <path=size | path.ico>...")
		os.Exit(2)
	}

	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "installericon:", err)
		os.Exit(1)
	}
}
