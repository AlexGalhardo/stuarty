package provider

import (
	"bytes"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Limits of SDD-016 sections 5.1 and 5.2 (epic 10).
const (
	MaxImagePrompt   = 4_000
	MaxImageCount    = 10
	MinImageSide     = 64
	MaxImageSide     = 4_096
	DefaultImageSize = "1024x1024"
	// DefaultVoiceChars is the longest text of a speech when the provider documents no limit.
	DefaultVoiceChars = 4_096
	MaxVoiceID        = 100
	MaxLanguage       = 35
	DefaultVoiceFmt   = "mp3"
)

// VoiceFormats are the audio formats a speech may be asked in; each is one FFmpeg decodes.
var VoiceFormats = []string{"mp3", "wav", "flac", "opus", "aac"}

var (
	voiceRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,99}$`)
	languageRe = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$`)
	sizeRe     = regexp.MustCompile(`^(\d{2,4})x(\d{2,4})$`)
)

// ImageRequest is one request for images (SDD-016 section 5.1).
type ImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
	Count  int    `json:"count,omitempty"`
}

// ImageLimits are what a provider documents for one model; a zero value is "not documented".
type ImageLimits struct {
	Sizes    []string
	MaxCount int
}

// VoiceRequest is one request for speech (SDD-016 section 5.2).
type VoiceRequest struct {
	Model    string `json:"model"`
	Text     string `json:"text"`
	Voice    string `json:"voice"`
	Language string `json:"language,omitempty"`
	Format   string `json:"format,omitempty"`
}

// VoiceLimits are what a provider documents for one model; a zero value is "not documented".
type VoiceLimits struct {
	MaxChars int
	Formats  []string
}

// NormalizeImage trims the model and the size and applies the defaults of size and count.
func NormalizeImage(r ImageRequest) ImageRequest {
	r.Model = strings.TrimSpace(r.Model)
	r.Size = strings.ToLower(strings.TrimSpace(r.Size))

	if r.Size == "" {
		r.Size = DefaultImageSize
	}

	if r.Count == 0 {
		r.Count = 1
	}

	return r
}

// ValidateImage checks a request, in stored form (NormalizeImage), against SDD-016 section 5.1 and the limits
// the adapter documents for the model.
func ValidateImage(r ImageRequest, l ImageLimits) *FieldError {
	if !ValidModel(r.Model) {
		return bad("model", "must be 1 to %d characters of letters, digits and . _ : / @ + -", MaxModelID)
	}

	if f := checkText("prompt", r.Prompt, 1, MaxImagePrompt); f != nil {
		return f
	}

	if f := validSize(r.Size, l.Sizes); f != nil {
		return f
	}

	maxCount := MaxImageCount
	if l.MaxCount > 0 {
		maxCount = min(maxCount, l.MaxCount)
	}

	if r.Count < 1 || r.Count > maxCount {
		return bad("count", "must be 1 to %d", maxCount)
	}

	return nil
}

func validSize(s string, allowed []string) *FieldError {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return bad("size", "must be the width and the height in pixels joined by x, such as %s", DefaultImageSize)
	}

	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])

	if w < MinImageSide || w > MaxImageSide || h < MinImageSide || h > MaxImageSide {
		return bad("size", "each side must be %d to %d pixels", MinImageSide, MaxImageSide)
	}

	if len(allowed) > 0 && !slices.Contains(allowed, s) {
		return bad("size", "the model makes only %s", strings.Join(allowed, ", "))
	}

	return nil
}

// NormalizeVoice trims the model, the voice and the language and applies the default format.
func NormalizeVoice(r VoiceRequest) VoiceRequest {
	r.Model = strings.TrimSpace(r.Model)
	r.Voice = strings.TrimSpace(r.Voice)
	r.Language = strings.TrimSpace(r.Language)
	r.Format = strings.ToLower(strings.TrimSpace(r.Format))

	if r.Format == "" {
		r.Format = DefaultVoiceFmt
	}

	return r
}

// ValidateVoice checks a request, in stored form (NormalizeVoice), against SDD-016 section 5.2 and the
// limits the adapter documents for the model.
func ValidateVoice(r VoiceRequest, l VoiceLimits) *FieldError {
	if !ValidModel(r.Model) {
		return bad("model", "must be 1 to %d characters of letters, digits and . _ : / @ + -", MaxModelID)
	}

	maxChars := DefaultVoiceChars
	if l.MaxChars > 0 {
		maxChars = l.MaxChars
	}

	if f := checkText("text", r.Text, 1, maxChars); f != nil {
		return f
	}

	if !voiceRe.MatchString(r.Voice) {
		return bad("voice", "must be 1 to %d characters of letters, digits and . _ : / @ + -", MaxVoiceID)
	}

	if r.Language != "" && (len(r.Language) > MaxLanguage || !languageRe.MatchString(r.Language)) {
		return bad("language", "must be a language tag such as pt-BR")
	}

	if !slices.Contains(VoiceFormats, r.Format) {
		return bad("format", "must be one of %s", strings.Join(VoiceFormats, ", "))
	}

	if len(l.Formats) > 0 && !slices.Contains(l.Formats, r.Format) {
		return bad("format", "the model writes only %s", strings.Join(l.Formats, ", "))
	}

	return nil
}

// ImageCost is the cost of count images at a price per image.
func ImageCost(count int, p Price) Cost {
	if count <= 0 || p.UnitMicros <= 0 {
		return Cost{Currency: p.Currency}
	}

	if p.UnitMicros > math.MaxInt64/int64(count) {
		return Cost{Micros: math.MaxInt64, Currency: p.Currency}
	}

	return Cost{Micros: int64(count) * p.UnitMicros, Currency: p.Currency}
}

// EstimateImage is ImageCost of the images the request asks for.
func EstimateImage(r ImageRequest, p Price) Cost { return ImageCost(r.Count, p) }

// VoiceCost is the cost of the characters of a text at a price per million characters, rounded up.
func VoiceCost(chars int, p Price) Cost {
	return Cost{Micros: mulDivCeil(int64(max(chars, 0)), p.UnitMicros), Currency: p.Currency}
}

// EstimateVoice is VoiceCost of the characters of the text of the request.
func EstimateVoice(r VoiceRequest, p Price) Cost {
	return VoiceCost(utf8.RuneCountInString(r.Text), p)
}

// CheckImage returns the format of a file that has the signature of a PNG, a JPEG or a WebP, and false for
// anything else (SDD-016 section 5.3).
func CheckImage(b []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "png", true
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg", true
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "webp", true
	}

	return "", false
}

// CheckAudio reports whether b has the signature of the audio format asked for (SDD-016 section 5.3).
func CheckAudio(b []byte, format string) bool {
	switch format {
	case "mp3":
		return bytes.HasPrefix(b, []byte("ID3")) || len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0 && b[1]&0x06 != 0
	case "wav":
		return len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WAVE"))
	case "flac":
		return bytes.HasPrefix(b, []byte("fLaC"))
	case "opus":
		return bytes.HasPrefix(b, []byte("OggS"))
	case "aac":
		return len(b) >= 2 && b[0] == 0xFF && b[1]&0xF6 == 0xF0
	}

	return false
}

// AudioExt is the file extension of a voice format.
func AudioExt(format string) string {
	if format == "opus" {
		return "ogg"
	}

	return format
}

// ImageExt is the file extension of a format CheckImage returned.
func ImageExt(format string) string {
	if format == "jpeg" {
		return "jpg"
	}

	return format
}
