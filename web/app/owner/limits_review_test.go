package owner

import (
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/engine/cache"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/events"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
)

// P4-SRV-003, SDD-007 section 9.1 and 6.5, SDD-000 section 10: every default of the table is the
// number in the code. A change to a default edits the SDD and this test together.
func TestLimitDefaultsAreTheOnesOfSDD007(t *testing.T) {
	l := service.Limits{}.WithDefaults()

	for name, c := range map[string][2]int64{
		"concurrent exports":              {int64(l.MaxExports), 2},
		"exports per key":                 {int64(l.MaxExportsPerActor), 1},
		"concurrent ingests":              {int64(l.MaxIngests), 2},
		"concurrent transcriptions":       {int64(l.MaxTranscribes), 1},
		"transcriptions per key":          {int64(l.MaxTranscribesPerActor), 1},
		"imported file size":              {l.MaxImportBytes, 8 << 30},
		"transcription seconds":           {l.MaxTranscribeSeconds, 7200},
		"export file size":                {l.MaxExportBytes, 4 << 30},
		"free disk":                       {l.MinFreeBytes, 2 << 30},
		"cache size":                      {cache.DefaultMaxBytes, 2 << 30},
		"source side":                     {ffmpeg.MaxSourceEdge, 8192},
		"source pixels":                   {ffmpeg.DefaultMaxPixels, 33_177_600},
		"source seconds":                  {int64(ffmpeg.DefaultMaxSeconds), 14400},
		"source streams":                  {ffmpeg.MaxStreams, 16},
		"source frame rate":               {ingest.MaxFrameRateRefused, 240},
		"animated image frames":           {ffmpeg.MaxAnimFrames, 1000},
		"animated image pixels":           {ffmpeg.MaxAnimPixels, 134_217_728},
		"requests per key a minute":       {httpapi.DefaultRatePerMinute, 300},
		"burst of a key":                  {httpapi.DefaultRateBurst, 60},
		"requests per session a minute":   {httpapi.DefaultSessionRatePerMinute, 1800},
		"burst of a session":              {httpapi.DefaultSessionRateBurst, 300},
		"heavy calls a minute":            {service.HeavyPerMinute, 10},
		"event streams per project":       {events.MaxPerProject, 32},
		"event streams per process":       {events.MaxPerProcess, 128},
		"event streams per key":           {events.MaxPerKey, 16},
		"output size poll interval (s)":   {int64(ffmpeg.OutputPollInterval / time.Second), 2},
		"cancel grace (s)":                {int64(ffmpeg.CancelGrace / time.Second), 5},
		"stall watchdog (s)":              {int64(ffmpeg.StallTimeout / time.Second), 120},
		"ffprobe deadline (s)":            {int64(ffmpeg.ProbeTimeout / time.Second), 30},
		"frame deadline (s)":              {int64(ffmpeg.FrameTimeout / time.Second), 30},
		"ffprobe output cap":              {ffmpeg.MaxProbeJSON, 4 << 20},
		"shutdown timeout (s)":            {int64(ShutdownTimeout / time.Second), 15},
		"ingest deadline, 0 s of media":   {int64(ffmpeg.IngestDeadline(0) / time.Second), 60},
		"export deadline, 0 s of project": {int64(ffmpeg.ExportDeadline(0) / time.Second), 120},
		"audio extraction, 0 s of audio":  {int64(ffmpeg.AudioExtractDeadline(0) / time.Second), 60},
		"transcription, 0 s of audio":     {int64(ffmpeg.TranscribeDeadline(0) / time.Second), 120},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, SDD-007 says %d", name, c[0], c[1])
		}
	}
}

// The multipliers and the ceilings of the deadlines of SDD-007 section 6.5.
func TestDeadlinesFollowTheMultipliersOfSDD007(t *testing.T) {
	const hour = time.Hour

	for name, c := range map[string][2]time.Duration{
		"ingest 10 min of media":      {ffmpeg.IngestDeadline(10 * time.Minute), 60*time.Second + 30*time.Minute},
		"ingest is capped at 4 h":     {ffmpeg.IngestDeadline(100 * hour), 4 * hour},
		"export 10 min":               {ffmpeg.ExportDeadline(10 * time.Minute), 120*time.Second + hour},
		"export is capped at 2 h":     {ffmpeg.ExportDeadline(100 * hour), 2 * hour},
		"audio 10 min":                {ffmpeg.AudioExtractDeadline(10 * time.Minute), 60*time.Second + 10*time.Minute},
		"audio is capped at 1 h":      {ffmpeg.AudioExtractDeadline(100 * hour), hour},
		"transcribe 10 min":           {ffmpeg.TranscribeDeadline(10 * time.Minute), 120*time.Second + 30*time.Minute},
		"transcribe is capped at 6 h": {ffmpeg.TranscribeDeadline(100 * hour), 6 * hour},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %v, SDD-007 says %v", name, c[0], c[1])
		}
	}
}
