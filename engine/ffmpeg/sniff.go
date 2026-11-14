package ffmpeg

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// SniffSize is how much of a file FF-05 reads before FFmpeg is started.
const SniffSize = 4096

// unsupported builds the E_UNSUPPORTED_MEDIA of SDD-011 5.3 without naming the file.
func unsupported(reason, message, hint string, details map[string]any) *Error {
	return &Error{Code: CodeUnsupportedMedia, Reason: reason, Message: message, Hint: hint, Details: details}
}

// refusedPrefixes are the text formats FF-05 names: playlists, concat scripts, subtitles
// and markup. They are refused whatever follows.
var refusedPrefixes = []string{"#EXTM3U", "ffconcat", "[Script Info]", "WEBVTT", "<?xml", "<"}

// Sniff applies FF-05 to the first bytes of a stored file: a known media signature is
// required, and text is refused without FFmpeg ever reading the file. head is at most
// SniffSize bytes.
func Sniff(head []byte) error {
	if len(head) > SniffSize {
		head = head[:SniffSize]
	}
	trimmed := bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	for _, p := range refusedPrefixes {
		if bytes.HasPrefix(trimmed, []byte(p)) {
			return notMedia()
		}
	}
	if mostlyPrintable(head) {
		return notMedia()
	}
	if knownSignature(head) {
		return nil
	}
	return notMedia()
}

// mostlyPrintable reports whether the first 512 bytes are text: a media header is binary,
// and a file that merely starts with a signature but reads as text is not trusted.
func mostlyPrintable(head []byte) bool {
	if len(head) > 512 {
		head = head[:512]
	}
	if len(head) < 16 {
		return false
	}
	printable := 0
	for _, c := range head {
		if c == '\n' || c == '\r' || c == '\t' || (c >= 0x20 && c < 0x7f) {
			printable++
		}
	}
	return printable*10 >= len(head)*9
}

func notMedia() *Error {
	return unsupported("unrecognized_format", "The file is not a media file FFmpeg can read.",
		"Import a video, audio or image file (MP4, MOV, MKV, WebM, AVI, MP3, WAV, FLAC, OGG, PNG, JPEG, WebP or GIF).",
		map[string]any{"ffprobe": "not a media signature"})
}

func knownSignature(h []byte) bool {
	n := len(h)
	at := func(off int, s string) bool { return n >= off+len(s) && string(h[off:off+len(s)]) == s }
	switch {
	case at(4, "ftyp"):
		return true
	case at(0, "\x1a\x45\xdf\xa3"):
		return true
	case at(0, "RIFF") && (at(8, "AVI ") || at(8, "WAVE") || at(8, "WEBP")):
		return true
	case at(0, "ID3"), at(0, "OggS"), at(0, "fLaC"), at(0, "GIF8"):
		return true
	case at(0, "\x00\x00\x01\xba"):
		return true
	case at(0, "FLV\x01"):
		return true
	case at(0, "\x89PNG\r\n\x1a\n"), at(0, "\xff\xd8\xff"):
		return true
	case n > 376 && h[0] == 0x47 && h[188] == 0x47 && h[376] == 0x47:
		return true
	case n >= 2 && h[0] == 0xff && h[1]&0xe0 == 0xe0:
		// MPEG audio frame sync or ADTS: eleven set bits. Layer 00 with version 1 is ADTS AAC,
		// 01 to 11 are layers III to I; the reserved version 01 is refused.
		return h[1]&0x18 != 0x08
	}
	return false
}

// SniffFile reads the first SniffSize bytes of path and applies [Sniff]. It is the check
// that runs before ffprobe for every stored original.
func SniffFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, SniffSize)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	return Sniff(buf[:n])
}
