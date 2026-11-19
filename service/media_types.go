package service

import (
	"time"

	"github.com/AlexGalhardo/stuarty/core/asset"
)

// ImportFile is one file of a MediaImport call (SDD-003 section 2.6): a path, or the hash of a file that the
// library already holds (SDD-011 section 14.3).
type ImportFile struct {
	Path    string `json:"path,omitempty"`     // absolute
	Hash    string `json:"hash,omitempty"`     // sha256:<64 hex> of a file of the library, in place of a path
	AssetID string `json:"asset_id,omitempty"` // optional caller-chosen id; ignored for a duplicate
}

// AssetInfo is what an import says about the files it brings, beyond their origin (SDD-011 section 14.4).
type AssetInfo struct {
	Tags        []string `json:"tags,omitempty"`
	Reusable    *bool    `json:"reusable,omitempty"`
	Description string   `json:"description,omitempty"`
}

// MediaImportRequest imports 1 to 50 files into a project.
type MediaImportRequest struct {
	ProjectID  string       `json:"project_id"`
	TimelineID string       `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	Files      []ImportFile `json:"files"`
	WaitMS     int          `json:"wait_ms,omitempty"` // 0 to 25000
	DryRun     bool         `json:"dry_run,omitempty"` // checks paths and limits, copies nothing
	// Origin says where the files came from; every file of the call shares it (SDD-011 section 14.2).
	Origin *asset.Origin `json:"origin,omitempty"`
	// Info is applied to the files of the call (SDD-011 section 14.4).
	Info *AssetInfo `json:"info,omitempty"`
}

// ProbeSummary is what the probe says about an asset.
type ProbeSummary struct {
	Kind       string     `json:"kind"`
	Container  string     `json:"container,omitempty"`
	Duration   *TimeValue `json:"duration,omitempty"` // in project frames; absent for an image
	Width      int        `json:"width,omitempty"`
	Height     int        `json:"height,omitempty"`
	FrameRate  string     `json:"frame_rate,omitempty"`
	VideoCodec string     `json:"video_codec,omitempty"`
	AudioCodec string     `json:"audio_codec,omitempty"`
	SampleRate int        `json:"sample_rate,omitempty"`
	Channels   int        `json:"channels,omitempty"`
	HasAudio   bool       `json:"has_audio"`
	SizeBytes  int64      `json:"size_bytes"`
	// Animated, Frames and LoopDurationMS describe an animated image (APNG, GIF): the number of frames
	// and the time of one pass over the delays of all of them, in milliseconds.
	Animated       bool  `json:"animated,omitempty"`
	Frames         int   `json:"frames,omitempty"`
	LoopDurationMS int64 `json:"loop_duration_ms,omitempty"`
}

// ImportedAsset is one entry per file of the request, in order. It never
// carries the directory of the path.
type ImportedAsset struct {
	AssetID string `json:"asset_id,omitempty"`
	// Hash is the content hash that the brand kit logo and font.add take (VV-117).
	Hash         string        `json:"hash,omitempty"`
	Name         string        `json:"name"             untrusted:"true"` // base name only; untrusted
	Kind         string        `json:"kind,omitempty"`
	State        string        `json:"state"`
	Progress     float64       `json:"progress"`
	JobID        string        `json:"job_id,omitempty"`
	Deduplicated bool          `json:"deduplicated"`
	Probe        *ProbeSummary `json:"probe,omitempty"`
	Error        *Error        `json:"error,omitempty"`

	// newInProject is true when this import created the asset in the project; MediaImport reads it to
	// tell a refused new file from a retry (VV-193).
	newInProject bool
}

// MediaImportResult answers MediaImport.
type MediaImportResult struct {
	Envelope
	ProjectID string          `json:"project_id"`
	Revision  int64           `json:"revision"`
	Assets    []ImportedAsset `json:"assets"`
	Failed    int             `json:"failed"`
	Imported  int             `json:"imported"`
	Text      string          `json:"text"`
}

// MediaListRequest lists every asset of a project.
type MediaListRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	Page
}

// Strip describes the sprite of timeline tiles of a video asset.
type Strip struct {
	TileW      int `json:"tile_w"`
	TileH      int `json:"tile_h"`
	Columns    int `json:"columns"`
	Count      int `json:"count"`
	IntervalMS int `json:"interval_ms"`
}

// MediaEntry is one asset of the list.
type MediaEntry struct {
	AssetID         string   `json:"asset_id"`
	Name            string   `json:"name"             untrusted:"true"` // base name; untrusted
	Kind            string   `json:"kind"`
	State           string   `json:"state"`
	Progress        float64  `json:"progress"`
	DurationFrames  *int64   `json:"duration_frames"`
	DurationSeconds *float64 `json:"duration_seconds"`
	Width           int      `json:"width,omitempty"`
	Height          int      `json:"height,omitempty"`
	HasAudio        bool     `json:"has_audio"`
	Trashed         bool     `json:"trashed"`
	Strip           *Strip   `json:"strip,omitempty"`
	Error           *Error   `json:"error,omitempty"`
}

// MediaListResult answers MediaList.
type MediaListResult struct {
	Envelope
	ProjectID  string       `json:"project_id"`
	Assets     []MediaEntry `json:"assets"`
	NextCursor *string      `json:"next_cursor"`
	Text       string       `json:"text"`
}

// MediaTrashRequest trashes or restores an asset of a project.
type MediaTrashRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	AssetID    string `json:"asset_id"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

// MediaStateResult is the state an asset ended in.
type MediaStateResult struct {
	AssetID string `json:"asset_id"`
	State   string `json:"state"` // "pending" | "active" | "trashed"
	Text    string `json:"text"`
}

// MediaInspectRequest reads one asset.
type MediaInspectRequest struct {
	ProjectID  string   `json:"project_id"`
	TimelineID string   `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	AssetID    string   `json:"asset_id"`
	Sections   []string `json:"sections,omitempty"` // default ["probe"]: probe, transcript, silences, loudness
	MinSilence string   `json:"min_silence,omitempty"`
	Page
}

// Loudness is the measured loudness of the audio of an asset.
type Loudness struct {
	IntegratedLUFS *float64 `json:"integrated_lufs"`
	RangeLU        float64  `json:"range_lu"`
	TruePeakDBTP   *float64 `json:"true_peak_dbtp"`
}

// BeatGrid is the detected beat grid of the audio of an asset (RF-044). An empty grid has TempoBPM 0 and
// a reason: audio without a steady pulse (speech, ambience) has no beats, and audio.mark_beats refuses it.
type BeatGrid struct {
	TempoBPM   float64 `json:"tempo_bpm"`
	Confidence float64 `json:"confidence"`
	Count      int     `json:"count"`
	// FirstMS are the first beats, in milliseconds of the asset.
	FirstMS []int64 `json:"first_ms"`
	Reason  string  `json:"reason,omitempty"`
}

// TimeRange is a range of project frames, the end exclusive.
type TimeRange struct {
	Start TimeValue `json:"start"`
	End   TimeValue `json:"end"`
}

// UntrustedData holds every string that came from the file (ADR-0020 item 5).
type UntrustedData struct {
	Name     string            `json:"name"               untrusted:"true"`
	Metadata map[string]string `json:"metadata,omitempty" untrusted:"true"`
	// Transcript is one page of the transcript, when the section was asked for.
	Transcript []TranscriptLine `json:"transcript,omitempty"`
}

// MediaInspectResult answers MediaInspect. An asset that is not ready answers
// with its state and progress instead of an error.
type MediaInspectResult struct {
	Envelope
	AssetID    string         `json:"asset_id"`
	Kind       string         `json:"kind"`
	State      string         `json:"state"`
	Progress   float64        `json:"progress"`
	Available  []string       `json:"available"`
	Strip      *Strip         `json:"strip,omitempty"`
	Probe      *ProbeSummary  `json:"probe,omitempty"`
	Silences   []TimeRange    `json:"silences,omitempty"`
	Loudness   *Loudness      `json:"loudness,omitempty"`
	Beats      *BeatGrid      `json:"beats,omitempty"`
	Untrusted  *UntrustedData `json:"untrusted,omitempty"`
	Error      *Error         `json:"error,omitempty"`
	NextCursor *string        `json:"next_cursor"`
	Text       string         `json:"text"`
}

// FileRef names a stored file for download (SDD-003 section 2.9).
type FileRef struct {
	Kind      string `json:"kind"` // "proxy", "chunks", "thumbnail", "strip", "peaks" or "bitmap"
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id"`                // the asset id
	Variant   string `json:"variant,omitempty"` // thumbnail width: 160, 320 or 640
}

// File is a stored file, resolved to a path inside the workspace.
type File struct {
	Path         string // slash path relative to the workspace
	DownloadName string // file name for Content-Disposition; empty means the job id
	ContentType  string
	Size         int64
	ETag         string // quoted
	ModTime      time.Time
	Immutable    bool
	MaxAge       int    // seconds of the Cache-Control header
	Body         []byte // the bytes to send when the file is built from the artifact (kind lut); Path is then not read
}
