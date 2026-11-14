package ingest

// Profile versions of the artifacts (SDD-011 1.5). A profile changes only by a new version,
// and the version is part of every artifact file name.
const (
	ProfileProbe      = 1
	ProfileNormalized = 1
	ProfileProxy      = 1
	ProfileThumbnails = 1
	ProfilePeaks      = 1
	ProfileSilence    = 1
	ProfileLoudness   = 1
)

// Tunable numbers of SDD-011 1.5. Changing one is a new profile version.
const (
	// MaxEdge is the longest edge of normalized video and images.
	MaxEdge = 3840
	// MaxFPS caps the nominal frame rate.
	MaxFPS = 60
	// NormalizedQScale is the -q:v of the mpeg4 encoder.
	NormalizedQScale = 2
	// ProxyShortEdge is the short edge of the proxy.
	ProxyShortEdge = 540
	// ProxyCRF is the libvpx-vp9 constant quality of the proxy.
	ProxyCRF = 33
	// ImageProxyEdge is the longest edge of an image proxy (SDD-011 4.7).
	ImageProxyEdge = 1280
	// ThumbHeight, ThumbMaxCount and ThumbColumns lay out the thumbnail sheet (4.5).
	ThumbHeight   = 128
	ThumbMaxCount = 400
	ThumbColumns  = 20
	// ThumbMaxTileWidth is the widest tile; wider frames are letterboxed.
	ThumbMaxTileWidth = 256
	// MaxFrameRateRefused is the measured rate above which a source is refused (SDD-007 9.1).
	MaxFrameRateRefused = 240
)

// PeakLevels are the samples per peak of the zoom levels (4.6).
var PeakLevels = [5]int{128, 512, 2048, 8192, 32768}

// SilenceThresholdsDB and SilenceMinMS are the settings of the silence artifact (9.1).
var SilenceThresholdsDB = [4]int{30, 35, 40, 45}

// SilenceMinMS is the shortest silence reported, in milliseconds.
const SilenceMinMS = 150

// SampleRate is the rate the project mixes at (SDD-001 3.1).
const SampleRate = 48000

// PosterWidths are the widths of the posters (4.5).
var PosterWidths = [3]int{160, 320, 640}
