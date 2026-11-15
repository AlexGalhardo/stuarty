package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

)

type recordingWord struct {
	Text    string `json:"text"`
	StartMS int    `json:"start_ms"`
	EndMS   int    `json:"end_ms"`
}

type recordingText struct {
	Size     string          `json:"size"`
	Seconds  int             `json:"seconds"`
	PausesMS [][2]int        `json:"pauses_ms"`
	Words    []recordingWord `json:"words"`
}

func readRecording(t *testing.T) recordingText {
	t.Helper()

	raw, err := os.ReadFile("fixtures/reference-2/recording.json")
	if err != nil {
		t.Fatal(err)
	}

	var rec recordingText
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}

	return rec
}

// talkingHead is the size, the length and the audio filter graph of the recording of
// reference scenario 2: a tone that goes silent during each pause of the committed text.
func talkingHead(t *testing.T) (size, seconds, audio string) {
	t.Helper()

	rec := readRecording(t)

	var mute []string

	for _, p := range rec.PausesMS {
		mute = append(mute, fmt.Sprintf("between(t,%.3f,%.3f)", float64(p[0])/1000, float64(p[1])/1000))
	}

	audio = fmt.Sprintf("sine=frequency=440:sample_rate=48000:duration=%d,volume=enable='%s':volume=0",
		rec.Seconds, strings.Join(mute, "+"))

	return rec.Size, fmt.Sprint(rec.Seconds), audio
}
