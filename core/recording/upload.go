package recording

import "github.com/AlexGalhardo/stuarty/core/timeline"

// The rules of a recording a browser captures and uploads (SDD-025 section 17). The browser holds none of them.

// SourceBrowser is the source kind of a recording that a browser captured and uploaded.
const SourceBrowser = "browser"

// The bounds of an upload.
const (
	// MaxChunkBytes is the largest body of one chunk: the limit of the upload route.
	MaxChunkBytes = 16 << 20
	// MaxChunksPerTrack bounds the index of a chunk; a recording of the longest allowed length at one chunk a
	// second is 14,400 chunks, so the bound is far above any honest upload.
	MaxChunksPerTrack = 100_000
	// MaxOpenUploadsPerActor and MaxOpenUploadsPerWorkspace bound the browser recordings that are open at once.
	MaxOpenUploadsPerActor     = 1
	MaxOpenUploadsPerWorkspace = 4
	// UploadIdleSeconds is how long a browser recording may receive nothing before the next start settles it.
	UploadIdleSeconds = 300
)

// Container is the file format a browser records.
type Container string

// The containers a browser records, and the formats the server remuxes them to.
const (
	ContainerWebM Container = "webm"
	ContainerMP4  Container = "mp4"
)

// Valid reports whether c is a container of an upload.
func (c Container) Valid() bool { return c == ContainerWebM || c == ContainerMP4 }

// UploadKind reports whether a track kind can be uploaded: the screen and the camera. Sound travels inside the
// file of the screen, mixed by the browser, because a browser records one stream per recorder.
func UploadKind(k TrackKind) bool { return k == TrackScreen || k == TrackCamera }

// File is the name of the file of an uploaded track once it is assembled: capture.webm, camera.mp4.
func (c Container) File(k TrackKind) string {
	switch k {
	case TrackScreen:
		return "capture." + string(c)
	case TrackCamera:
		return "camera." + string(c)
	case TrackMicrophone, TrackSystem:
	}

	return ""
}

// UploadSpec is what a person asks for when a browser recording begins.
type UploadSpec struct {
	ProjectID string
	Container string
	// Camera says that a second track, the camera, will be uploaded.
	Camera bool
}

// ValidateUpload checks a request to begin an upload and returns every refused field.
func ValidateUpload(s UploadSpec) []FieldError {
	var out []FieldError

	if !timeline.ValidID("prj", s.ProjectID) {
		out = append(out, FieldError{"project_id", "invalid_id"})
	}

	if !Container(s.Container).Valid() {
		out = append(out, FieldError{"container", "not_allowed"})
	}

	return out
}

// UploadTracks is the tracks an upload asks for, with the file each will have: the screen, then the camera.
func UploadTracks(s UploadSpec) []Track {
	c := Container(s.Container)
	out := []Track{{Kind: TrackScreen, File: c.File(TrackScreen)}}

	if s.Camera {
		out = append(out, Track{Kind: TrackCamera, File: c.File(TrackCamera)})
	}

	return out
}

// ValidSHA256 reports whether s is a SHA-256 digest in lowercase hexadecimal, the form a chunk declares.
func ValidSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}

	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}

	return true
}

// ChunkVerdict is what the order of a chunk allows.
type ChunkVerdict string

// The verdicts of JudgeChunk.
const (
	// ChunkAccept is the chunk the track waits for.
	ChunkAccept ChunkVerdict = "accept"
	// ChunkRetry is an index already stored: the same chunk sent again, which is accepted if its digest matches.
	ChunkRetry ChunkVerdict = "retry"
	// ChunkOutOfOrder is a gap: an index above the one the track waits for.
	ChunkOutOfOrder ChunkVerdict = "out_of_order"
	// ChunkTooMany is an index at or above MaxChunksPerTrack.
	ChunkTooMany ChunkVerdict = "too_many"
	// ChunkMalformed is a negative index, which no chunk has (SDD-025 section 17.4 item 2).
	ChunkMalformed ChunkVerdict = "malformed"
)

// JudgeChunk decides what the index of a chunk means when the track has stored chunks 0 to next-1.
func JudgeChunk(next, index int) ChunkVerdict {
	switch {
	case index < 0:
		return ChunkMalformed
	case index >= MaxChunksPerTrack:
		return ChunkTooMany
	case index < next:
		return ChunkRetry
	case index > next:
		return ChunkOutOfOrder
	}

	return ChunkAccept
}

// FitsAfter reports whether a chunk of n bytes keeps the stored bytes within the byte limit.
func (l Limits) FitsAfter(storedBytes, n int64) bool { return storedBytes+n <= l.MaxBytes }
