package gioapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Gallery writes the screenshots of the desktop editor into dir.
func (ui *UI) Gallery(dir string) error {
	if ui.sess == nil {
		return errors.New("the workspace is not open")
	}

	ui.useTheme(false, false)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	ctx := context.Background()
	created, err := ui.sess.Create(ctx, "Launch teaser", "16:9")
	if err != nil {
		return err
	}

	if err := ui.reloadSync(ctx, created.ProjectID); err != nil {
		return err
	}

	if err := ui.shoot(dir, "desktop-editor-empty.png"); err != nil {
		return err
	}

	list, err := ui.sess.Projects(ctx)
	if err != nil {
		return err
	}

	ui.projects = list
	ui.HideProject()

	if err := ui.shoot(dir, "desktop-projects.png"); err != nil {
		return err
	}

	if err := ui.reloadSync(ctx, created.ProjectID); err != nil {
		return err
	}

	clip, err := sampleClip(ctx)
	if err != nil {
		return err
	}

	if err := ui.importClip(ctx, clip); err != nil {
		return err
	}

	ui.selected = ""
	ui.boundSelection = ""

	if ui.stage != nil {
		_ = ui.stage.ShowSync(ui.doc, ui.revision, 0)
	}

	if err := ui.shoot(dir, "desktop-editor-clips.png"); err != nil {
		return err
	}

	if err := ui.shoot(dir, "desktop-editor-clips-dark.png"); err != nil {
		return err
	}

	ui.useTheme(true, false)

	if err := ui.shoot(dir, "desktop-editor-clips-light.png"); err != nil {
		return err
	}

	list, err = ui.sess.Projects(ctx)
	if err != nil {
		return err
	}

	ui.projects = list
	ui.HideProject()

	if err := ui.shoot(dir, "desktop-projects-light.png"); err != nil {
		return err
	}

	if err := ui.reloadSync(ctx, created.ProjectID); err != nil {
		return err
	}

	ui.useTheme(true, false)
	ui.selected = clipItem(ui.doc)
	ui.boundSelection = ""

	if ui.stage != nil {
		_ = ui.stage.ShowSync(ui.doc, ui.revision, 0)
	}

	if err := ui.shoot(dir, "desktop-inspector-light.png"); err != nil {
		return err
	}

	ui.useTheme(false, false)
	ui.selected = clipItem(ui.doc)
	ui.boundSelection = ""

	if err := ui.shoot(dir, "desktop-inspector.png"); err != nil {
		return err
	}

	ui.exportOpen = true

	if err := ui.shoot(dir, "desktop-export.png"); err != nil {
		return err
	}

	ui.exportOpen = false
	ui.selected = clipItem(ui.doc)

	if ui.stage != nil {
		ui.stage.play(ui.doc, ui.revision, 0)

		if !ui.stage.WaitUntil(15, 8*time.Second) {
			return fmt.Errorf("playback did not reach frame 15 (%s)", ui.stage.snapshot().err)
		}
	}

	ui.playing = true

	return ui.shoot(dir, "desktop-preview-playing.png")
}

func (ui *UI) shoot(dir, name string) error {
	return RenderPNG(ui, filepath.Join(dir, name), 1600, 900)
}

func (ui *UI) importClip(ctx context.Context, path string) error {
	res, err := ui.sess.Import(ctx, ui.projectID, []string{path})
	if err != nil {
		return err
	}

	if res.Failed > 0 || len(res.Assets) == 0 || res.Assets[0].AssetID == "" {
		msg := "the sample clip was not imported"
		if len(res.Assets) > 0 && res.Assets[0].Error != nil {
			msg = res.Assets[0].Error.Error()
		}

		return errors.New(msg)
	}

	if err := ui.reloadSync(ctx, ui.projectID); err != nil {
		return err
	}

	track := timeline.NewID(timeline.PrefixTrack)
	add, err := operation("track.add", map[string]any{"kind": "visual", "id": track, "name": "Video"})
	if err != nil {
		return err
	}

	place, err := operation("clip.add", map[string]any{
		"track_id": track, "asset_id": res.Assets[0].AssetID, "start": "0s",
	})
	if err != nil {
		return err
	}

	title, err := operation("text.add", map[string]any{
		"start": "0s", "duration": "2s", "text": "Launch",
	})
	if err != nil {
		return err
	}

	if _, err := ui.sess.Edit(ctx, ui.projectID, ui.revision, add, place, title); err != nil {
		return err
	}

	return ui.reloadSync(ctx, ui.projectID)
}

func clipItem(doc *timeline.Document) string {
	if doc == nil {
		return ""
	}

	for id := range doc.Items {
		if doc.Items[id].Type == timeline.ItemClip {
			return id
		}
	}

	return ""
}

func sampleClip(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp("", "vukem-sample-")
	if err != nil {
		return "", err
	}

	out := filepath.Join(dir, "clip.mp4")
	cmd := exec.CommandContext(
		ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=s=640x360:r=30:d=3",
		"-f", "lavfi", "-i", "sine=f=440:r=44100:d=3",
		"-c:v", "mpeg4", "-dct", "int", "-q:v", "4", "-c:a", "aac", "-f", "mp4", out,
	)

	if raw, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("sample clip: %w: %s", err, raw)
	}

	return out, nil
}

// WaitUntil reports whether playback has drawn a frame at or after frame.
func (st *stage) WaitUntil(frame int, d time.Duration) bool {
	deadline := time.Now().Add(d)

	for time.Now().Before(deadline) {
		snap := st.snapshot()
		if snap.img != nil && snap.at >= frame {
			return true
		}

		time.Sleep(20 * time.Millisecond)
	}

	return false
}
