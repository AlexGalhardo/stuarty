// Package export holds the pure rules of exporting a video for a network: the table of presets
// with the source and the date of every limit, the check of a project against a preset, the codec
// names and the bit rate a preset asks for (SDD-004 sections 8.2 and 8.3, SDD-002 A.13). It never
// opens a file and never starts a process.
package export

import "slices"

// Codec is the video codec of an export. The zero value is H.264.
type Codec string

// The codecs of an export (SDD-000 section 15).
const (
	CodecH264 Codec = "h264"
	CodecHEVC Codec = "hevc"
	CodecAV1  Codec = "av1"
)

// Codecs lists every codec, H.264 first.
func Codecs() []Codec { return []Codec{CodecH264, CodecHEVC, CodecAV1} }

// Valid says whether c is a codec of an export; the empty string is H.264.
func (c Codec) Valid() bool { return c == "" || slices.Contains(Codecs(), c) }

// Normal is the codec with the default filled in.
func (c Codec) Normal() Codec {
	if c == "" {
		return CodecH264
	}

	return c
}

// The share of the H.264 bit rate that each codec takes for a picture of about the same quality
// (SDD-004 section 8.2): a product choice, never a promise.
const (
	hevcPermille = 600
	av1Permille  = 500
	// aboveThirtyPermille raises the rate of a project faster than 30 fps (YouTube asks 12 against 8).
	aboveThirtyPermille = 1500
	// MinVideoKbps is the floor of the video bit rate a preset asks for.
	MinVideoKbps = 1000
)

// Ratio is a fraction of two integers; the zero value means "no limit".
type Ratio struct{ Num, Den int }

// Limits are what a network refuses, as the pages in Preset.Sources state them. A zero field is a
// limit the page does not state, and is never checked.
type Limits struct {
	MinDurationMS, MaxDurationMS int64
	// MinFPS and MaxFPS are whole frames per second.
	MinFPS, MaxFPS      int
	MinWidth, MinHeight int
	MaxWidth, MaxHeight int
	// AspectMin and AspectMax are the range of width over height; AspectTolerancePermille widens both ends.
	AspectMin, AspectMax    Ratio
	AspectTolerancePermille int
	MaxBytes                int64
	// MaxVideoKbps is the video bit rate a preset never asks above.
	MaxVideoKbps int
}

// Targets are what an export with the preset asks of the encoders: the recommended size, and the
// bit rates for it at up to 30 fps with H.264.
type Targets struct {
	Width, Height int
	VideoKbps     int
	AudioKbps     int
	SampleHz      int
	Channels      int
}

// Source is the official page a limit was read on, and when.
type Source struct {
	URL string `json:"url"`
	// Read is the date the page was read, YYYY-MM-DD.
	Read string `json:"read"`
	// Says is what the page states, in a few words.
	Says string `json:"says"`
}

// Preset is what a network asks of a video (RF-087).
type Preset struct {
	ID        string
	Network   string
	Placement string
	Label     string
	// Container is always mp4.
	Container string
	// Codecs is the preference order; every codec of it is one the source says the network takes.
	Codecs  []Codec
	Limits  Limits
	Targets Targets
	Sources []Source
}

// Presets returns the presets of the product, in the order of SDD-000 section 15. The result is a
// copy: changing it changes nothing.
func Presets() []Preset {
	out := make([]Preset, len(table))
	for i := range table {
		out[i] = table[i].clone()
	}

	return out
}

func (p *Preset) clone() Preset {
	c := *p
	c.Codecs = slices.Clone(p.Codecs)
	c.Sources = slices.Clone(p.Sources)

	return c
}

// Lookup returns the preset with the id.
func Lookup(id string) (Preset, bool) {
	for i := range table {
		if table[i].ID == id {
			return table[i].clone(), true
		}
	}

	return Preset{}, false
}

// IDs are the preset ids in order.
func IDs() []string {
	ids := make([]string, len(table))
	for i := range table {
		ids[i] = table[i].ID
	}

	return ids
}

// Read dates of the pages of the table.
const readOn = "2026-11-19"

const (
	srcYouTubeShorts = "https://support.google.com/youtube/answer/15424877"
	srcYouTubeEncode = "https://support.google.com/youtube/answer/1722171"
	srcInstagram     = "https://developers.facebook.com/documentation/instagram-platform/instagram-graph-api/reference/ig-user/media"
	srcFacebook      = "https://developers.facebook.com/docs/video-api/guides/reels-publishing/"
	srcTikTok        = "https://developers.tiktok.com/doc/content-posting-api-media-transfer-guide"
	srcXBest         = "https://docs.x.com/x-api/media/quickstart/best-practices.md"
	srcXUpload       = "https://docs.x.com/x-api/media/quickstart/media-upload-chunked"
)

const (
	mb = 1_000_000
	gb = 1_000_000_000
)

// table is the data of SDD-004 section 8.2. Every limit was read on the page named beside it; a
// limit a page does not state is zero.
var table = []Preset{
	{
		ID: "youtube-shorts", Network: "YouTube", Placement: "Shorts", Label: "YouTube Shorts",
		Container: "mp4", Codecs: []Codec{CodecH264},
		Limits: Limits{
			MaxDurationMS: 180_000,
			AspectMax:     Ratio{1, 1},
		},
		Targets: Targets{Width: 1080, Height: 1920, VideoKbps: 8000, AudioKbps: 384, SampleHz: 48000, Channels: 2},
		Sources: []Source{
			{srcYouTubeShorts, readOn, "Shorts last up to three minutes; a square or vertical video is a Short"},
			{
				srcYouTubeEncode,
				readOn,
				"H.264 in MP4 with moov first, AAC-LC, 8 Mbit/s at 1080p up to 30 fps and 12 above, 384 kbit/s stereo",
			},
		},
	},
	{
		ID: "instagram-reels", Network: "Instagram", Placement: "Reels", Label: "Instagram Reels",
		Container: "mp4", Codecs: []Codec{CodecHEVC, CodecH264},
		Limits: Limits{
			MinDurationMS: 3_000, MaxDurationMS: 900_000,
			MinFPS: 23, MaxFPS: 60,
			MaxWidth:  1920,
			AspectMin: Ratio{1, 100}, AspectMax: Ratio{10, 1},
			MaxBytes: 300 * mb, MaxVideoKbps: 25_000,
		},
		Targets: Targets{Width: 1080, Height: 1920, VideoKbps: 8000, AudioKbps: 128, SampleHz: 48000, Channels: 2},
		Sources: []Source{
			{
				srcInstagram,
				readOn,
				"Reel specifications: MOV or MP4, HEVC or H.264, AAC up to 48 kHz, 23 to 60 fps, VBR 25 Mbit/s, 3 s to 15 min, 300 MB, width up to 1920, aspect 0.01:1 to 10:1",
			},
		},
	},
	{
		ID: "facebook-reels", Network: "Facebook", Placement: "Reels", Label: "Facebook Reels",
		Container: "mp4", Codecs: []Codec{CodecHEVC, CodecH264},
		Limits: Limits{
			MinDurationMS: 3_000, MaxDurationMS: 90_000,
			MinFPS: 24, MaxFPS: 60,
			MinWidth: 540, MinHeight: 960,
			AspectMin: Ratio{9, 16}, AspectMax: Ratio{9, 16}, AspectTolerancePermille: 10,
		},
		Targets: Targets{Width: 1080, Height: 1920, VideoKbps: 8000, AudioKbps: 192, SampleHz: 48000, Channels: 2},
		Sources: []Source{
			{
				srcFacebook,
				readOn,
				".mp4, H.264 or H.265, 24 to 60 fps, 9x16, 1080x1920 recommended and 540x960 minimum, 3 to 90 s, AAC-LC 48 kHz stereo 128 kbit/s or more",
			},
		},
	},
	{
		ID: "tiktok", Network: "TikTok", Placement: "Video", Label: "TikTok",
		Container: "mp4", Codecs: []Codec{CodecHEVC, CodecH264},
		Limits: Limits{
			MaxDurationMS: 600_000,
			MinFPS:        23, MaxFPS: 60,
			MinWidth: 360, MinHeight: 360, MaxWidth: 4096, MaxHeight: 4096,
			MaxBytes: 4 * gb,
		},
		Targets: Targets{Width: 1080, Height: 1920, VideoKbps: 8000, AudioKbps: 192, SampleHz: 48000, Channels: 2},
		Sources: []Source{
			{
				srcTikTok,
				readOn,
				"MP4, WebM or MOV; H.264 recommended and H.265 supported; 23 to 60 fps; 360 to 4096 pixels per side; 4 GB; up to 10 minutes through the upload endpoint",
			},
		},
	},
	{
		ID: "x-video", Network: "X", Placement: "Post video", Label: "X video",
		Container: "mp4", Codecs: []Codec{CodecH264},
		Limits: Limits{
			MinDurationMS: 500, MaxDurationMS: 1_200_000,
			MaxFPS:    60,
			AspectMin: Ratio{1, 3}, AspectMax: Ratio{3, 1},
			MaxBytes: 8 * gb,
		},
		Targets: Targets{Width: 720, Height: 1280, VideoKbps: 5000, AudioKbps: 128, SampleHz: 48000, Channels: 2},
		Sources: []Source{
			{
				srcXBest,
				readOn,
				"H.264 High, 4:2:0, AAC-LC, at most 60 fps, aspect 1:3 to 3:1, 5,000 kbit/s video and 128 kbit/s audio; its 1280x1024 limit contradicts the sizes it recommends and is not enforced",
			},
			{srcXUpload, readOn, "a post video lasts up to 20 minutes and 8 GB by default"},
		},
	},
}
