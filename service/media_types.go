package service

import "time"

// ImportFile is one path of a MediaImport call (SDD-003 section 2.6).
type ImportFile struct {
	Path    string `json:"path"`               // absolute
	AssetID string `json:"asset_id,omitempty"` // optional caller-chosen id; ignored for a duplicate
}

// MediaImportRequest imports 1 to 50 files into a project.
type MediaImportRequest struct {
	ProjectID string       `json:"project_id"`
	Files     []ImportFile `json:"files"`
	WaitMS    int          `json:"wait_ms,omitempty"` // 0 to 25000
	DryRun    bool         `json:"dry_run,omitempty"` // checks paths and limits, copies nothing
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
}

// ImportedAsset is one entry per file of the request, in order. It never
// carries the directory of the path.
type ImportedAsset struct {
	AssetID      string        `json:"asset_id,omitempty"`
	Name         string        `json:"name"               untrusted:"true"` // base name only; untrusted
	Kind         string        `json:"kind,omitempty"`
	State        string        `json:"state"`
	Progress     float64       `json:"progress"`
	JobID        string        `json:"job_id,omitempty"`
	Deduplicated bool          `json:"deduplicated"`
	Probe        *ProbeSummary `json:"probe,omitempty"`
	Error        *Error        `json:"error,omitempty"`
}

// MediaImportResult answers MediaImport.
type MediaImportResult struct {
	Envelope
	ProjectID string          `json:"project_id"`
	Revision  int64           `json:"revision"`
	Assets    []ImportedAsset `json:"assets"`
	Failed    int             `json:"failed"`
	Text      string          `json:"text"`
}

// MediaListRequest lists every asset of a project.
type MediaListRequest struct {
	ProjectID string `json:"project_id"`
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
	ProjectID string `json:"project_id"`
	AssetID   string `json:"asset_id"`
	DryRun    bool   `json:"dry_run,omitempty"`
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
	Path        string // slash path relative to the workspace
	ContentType string
	Size        int64
	ETag        string // quoted
	ModTime     time.Time
	Immutable   bool
	MaxAge      int // seconds of the Cache-Control header
}
