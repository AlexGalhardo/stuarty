package publish

// The limits of the networks of epic 22, as data (SDD-020 sections 32 and 35). The numbers of length, frame
// rate, picture and size of the video come from the presets of core/export; what is here is what the preset
// does not carry: the formats, the codecs, the text and the visibility.

const (
	xSource      = "https://docs.x.com/x-api/media/quickstart/best-practices.md"
	tikTokSource = "https://developers.tiktok.com/doc/content-posting-api-media-transfer-guide"
	networksRead = "2026-11-20"
	// gb is the unit the presets use for a size in gigabytes.
	gb = 1_000_000_000
)

// XLimits are the limits of a post to X for an account with no X Premium: the text takes 280 weighted
// characters, a link weighing 23, and the video at most 8 GB and 20 minutes (the preset x-video). The limit of
// 1,280 x 1,024 pixels of the page is not enforced: the same page recommends 720 x 1,280.
func XLimits() Limits {
	return Limits{
		TitleMax: 1000, DescriptionMaxBytes: 4000, TagsMaxChars: 1000,
		FileMaxBytes: 8 * gb,
		Formats:      []string{"mp4"},
		// A post to X has no thumbnail in this epic: the list is empty, so a thumbnail is refused.
		Visibilities: []Visibility{Public},
		Preset:       "x-video", VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"},
		CaptionNetwork: "x",
		Source:         xSource, Read: networksRead,
	}
}

// TikTokLimits are the limits of the video the draft of TikTok takes (the preset tiktok). TikTok takes the
// file and nothing else: no caption, no privacy level, no cover, no AI flag. The only visibility is private,
// which is also the default, so nothing is public until the person finishes the draft in TikTok.
func TikTokLimits() Limits {
	return Limits{
		TitleMax: 100, FileMaxBytes: 4 * gb,
		Formats:      []string{"mp4"},
		Visibilities: []Visibility{Private},
		Preset:       "tiktok", VideoCodecs: []string{"h264", "hevc"},
		DailyPosts: 5, NoCaption: true,
		Source: tikTokSource, Read: networksRead,
	}
}
