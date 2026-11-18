package gioapp

import (
	"context"
	"errors"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func (ui *UI) edit(ops ...command.Operation) {
	if ui.doc == nil || ui.inflight || len(ops) == 0 {
		return
	}

	id, rev := ui.projectID, ui.revision
	ui.async("Applying edit…", func(ctx context.Context) error {
		if _, err := ui.sess.Edit(ctx, id, rev, ops...); err != nil {
			return err
		}

		return ui.reload(ctx, id)
	})
}

func (ui *UI) createProject() {
	name := strings.TrimSpace(ui.nameEd.Text())
	if name == "" {
		name = "Untitled"
	}

	preset := ui.preset
	if preset == "" {
		preset = "16:9"
	}

	ui.async("Creating project…", func(ctx context.Context) error {
		res, err := ui.sess.Create(ctx, name, preset)
		if err != nil {
			return err
		}

		if err := ui.reload(ctx, res.ProjectID); err != nil {
			return err
		}

		list, err := ui.sess.Projects(ctx)
		if err != nil {
			return err
		}

		ui.post(func() { ui.projects = list })

		return nil
	})
}

func (ui *UI) openProject(id string) {
	ui.async("Opening project…", func(ctx context.Context) error {
		return ui.reload(ctx, id)
	})
}

func (ui *UI) closeProject() {
	ui.playing = false
	ui.projectID = ""
	ui.doc = nil
	ui.selected = ""
	ui.frame = frameView{}
	ui.async("Loading projects…", func(ctx context.Context) error {
		list, err := ui.sess.Projects(ctx)
		if err != nil {
			return err
		}

		ui.post(func() { ui.projects = list })

		return nil
	})
}

func (ui *UI) undo() {
	if ui.doc == nil {
		return
	}

	id := ui.projectID
	ui.async("Undoing…", func(ctx context.Context) error {
		if _, err := ui.sess.Undo(ctx, id); err != nil {
			return err
		}

		return ui.reload(ctx, id)
	})
}

func (ui *UI) redo() {
	if ui.doc == nil {
		return
	}

	id := ui.projectID
	ui.async("Redoing…", func(ctx context.Context) error {
		if _, err := ui.sess.Redo(ctx, id); err != nil {
			return err
		}

		return ui.reload(ctx, id)
	})
}

func (ui *UI) trackID(kind timeline.TrackKind) (string, command.Operation, bool) {
	if ui.doc != nil {
		for id, tr := range ui.doc.Tracks {
			if tr.Kind == kind {
				return id, command.Operation{}, false
			}
		}
	}

	id := timeline.NewID(timeline.PrefixTrack)
	name := "Video"
	kindName := "visual"

	if kind == timeline.TrackAudio {
		name = "Audio"
		kindName = "audio"
	}

	op, err := operation("track.add", map[string]any{"kind": kindName, "id": id, "name": name})
	if err != nil {
		ui.status = err.Error()

		return "", command.Operation{}, false
	}

	return id, op, true
}

func (ui *UI) placeAsset(id string, asAudio bool) {
	a := ui.assetByID(id)
	if a == nil {
		ui.status = "Select a file in the bin."

		return
	}

	if a.State != "" && a.State != "active" {
		ui.status = "That file is still importing."

		return
	}

	audio := asAudio || a.Kind == string(timeline.AssetAudio)
	kind := timeline.TrackVisual
	cmd := "clip.add"

	if audio {
		if a.Kind == string(timeline.AssetImage) || a.Kind == string(timeline.AssetLUT) {
			ui.status = "That file has no audio to place."

			return
		}

		kind = timeline.TrackAudio
		cmd = "audio.add"
	} else if a.Kind == string(timeline.AssetAudio) || a.Kind == string(timeline.AssetLUT) {
		ui.status = "Place audio from the Audio tab."

		return
	}

	track, extra, ok := ui.trackID(kind)
	if track == "" {
		return
	}

	op, err := operation(cmd, map[string]any{
		"track_id": track, "asset_id": a.ID, "start": framesArg(ui.playhead),
	})
	if err != nil {
		ui.status = err.Error()

		return
	}

	if ok {
		ui.edit(extra, op)

		return
	}

	ui.edit(op)
}

func (ui *UI) addText(size int) {
	text := strings.TrimSpace(ui.draftEd.Text())
	if text == "" {
		text = "Title"
	}

	op, err := operation("text.add", map[string]any{
		"start": framesArg(ui.playhead), "duration": "3s", "text": text,
		"style": map[string]any{"size": size, "color": "#FFFFFF"},
	})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) addShape() {
	op, err := operation("shape.add", map[string]any{
		"start": framesArg(ui.playhead), "duration": "3s", "shape": "rounded_rect",
		"width": 640, "height": 120, "fill": "#22D3EE",
	})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) split() {
	it := ui.item(ui.selected)
	if it == nil {
		ui.status = "Select a clip to split."

		return
	}

	if ui.playhead <= it.Start || ui.playhead >= it.Start+it.Duration {
		ui.status = "Move the playhead inside the clip to split it."

		return
	}

	op, err := operation("item.split", map[string]any{"item_id": it.ID, "at": framesArg(ui.playhead)})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) deleteSelected() {
	if ui.selected == "" {
		ui.status = "Select a clip to delete."

		return
	}

	op, err := operation("item.delete", map[string]any{"item_ids": []string{ui.selected}})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.selected = ""
	ui.edit(op)
}

func (ui *UI) applyLabel() {
	it := ui.item(ui.selected)
	if it == nil {
		return
	}

	op, err := operation("item.rename", map[string]any{"item_id": it.ID, "name": strings.TrimSpace(ui.labelEd.Text())})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) applyText() {
	it := ui.item(ui.selected)
	if it == nil || (it.Type != timeline.ItemText && it.Type != timeline.ItemCaption) {
		return
	}

	text := strings.TrimSpace(ui.textEd.Text())
	if text == "" {
		ui.status = "Text cannot be empty."

		return
	}

	op, err := operation("text.set", map[string]any{"item_id": it.ID, "text": text})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) setTransform(key string, value float64) {
	it := ui.item(ui.selected)
	if it == nil || it.Type == timeline.ItemAudio {
		ui.status = "Select a visual clip."

		return
	}

	op, err := operation("item.set_transform", map[string]any{"item_id": it.ID, key: value})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) setVolume(v float64) {
	it := ui.item(ui.selected)
	if it == nil || (it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio) {
		ui.status = "Select a clip with audio."

		return
	}

	op, err := operation("item.set_audio", map[string]any{"item_id": it.ID, "volume": v})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) setSpeed(s string) {
	it := ui.item(ui.selected)
	if it == nil || (it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio) {
		ui.status = "Select a clip to change its speed."

		return
	}

	op, err := operation("clip.set_speed", map[string]any{"item_id": it.ID, "speed": s})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) setFit(fit string) {
	it := ui.item(ui.selected)
	if it == nil || it.Type != timeline.ItemClip {
		ui.status = "Fit applies to a video or image clip."

		return
	}

	op, err := operation("clip.set_framing", map[string]any{"item_id": it.ID, "fit": fit})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) setFilter(name string) {
	it := ui.item(ui.selected)
	if it == nil || it.Type == timeline.ItemAudio {
		ui.status = "Select a visual clip."

		return
	}

	op, err := operation("clip.set_filter", map[string]any{"item_id": it.ID, "filter": name})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) setBlur(radius float64) {
	it := ui.item(ui.selected)
	if it == nil || it.Type == timeline.ItemAudio {
		return
	}

	op, err := operation("clip.set_blur", map[string]any{"item_id": it.ID, "radius": radius})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) fadeIn() {
	it := ui.item(ui.selected)
	if it == nil || it.Type == timeline.ItemAudio {
		ui.status = "Select a visual clip."

		return
	}

	op, err := operation("item.set_transition", map[string]any{
		"item_id": it.ID, "edge": "in", "kind": "fade", "duration": "0.4s",
	})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) setColor(brightness float64) {
	it := ui.item(ui.selected)
	if it == nil || it.Type != timeline.ItemClip {
		ui.status = "Colour applies to a video or image clip."

		return
	}

	op, err := operation("clip.set_color", map[string]any{"item_id": it.ID, "brightness": brightness})
	if err != nil {
		ui.status = err.Error()

		return
	}

	ui.edit(op)
}

func (ui *UI) commitDrag() {
	d := ui.drag
	ui.drag = dragState{}

	if !d.active || d.delta == 0 || d.item == "" {
		return
	}

	start := d.originStart
	dur := d.originDur

	switch d.kind {
	case hitBody:
		start += d.delta
		if start < 0 {
			start = 0
		}

		op, err := operation("item.move", map[string]any{"item_id": d.item, "start": framesArg(start)})
		if err != nil {
			ui.status = err.Error()

			return
		}

		ui.edit(op)
	case hitStart:
		start += d.delta
		if start < 0 {
			start = 0
		}

		if start >= d.originStart+dur {
			start = d.originStart + dur - 1
		}

		op, err := operation("item.trim", map[string]any{"item_id": d.item, "edge": "start", "to": framesArg(start)})
		if err != nil {
			ui.status = err.Error()

			return
		}

		ui.edit(op)
	case hitEnd:
		end := d.originStart + dur + d.delta
		if end <= d.originStart {
			end = d.originStart + 1
		}

		op, err := operation("item.trim", map[string]any{"item_id": d.item, "edge": "end", "to": framesArg(end)})
		if err != nil {
			ui.status = err.Error()

			return
		}

		ui.edit(op)
	default:
	}
}

func (ui *UI) displaySpan(it timeline.Item) (start, dur int) {
	start, dur = it.Start, it.Duration
	if !ui.drag.active || ui.drag.item != it.ID || ui.drag.delta == 0 {
		return start, dur
	}

	switch ui.drag.kind {
	case hitBody:
		start = max(start+ui.drag.delta, 0)
	case hitStart:
		next := max(start+ui.drag.delta, 0)
		if next >= start+dur {
			next = start + dur - 1
		}

		dur -= next - start
		start = next
	case hitEnd:
		dur = max(dur+ui.drag.delta, 1)
	default:
	}

	return start, dur
}

// SeedDemo creates a 16:9 project with text, a shape and an audio track, and opens it.
func (ui *UI) SeedDemo(ctx context.Context) error {
	if ui.sess == nil {
		return errors.New("the workspace is not open")
	}

	created, err := ui.sess.Create(ctx, "Launch teaser", "16:9")
	if err != nil {
		return err
	}

	specs := []struct {
		name string
		args any
	}{
		{"track.add", map[string]any{"kind": "audio", "name": "Audio"}},
		{"text.add", map[string]any{
			"start": "0s", "duration": "4s", "text": "Vukem Video",
			"style": map[string]any{"size": 120, "color": "#FFFFFF"},
		}},
		{"shape.add", map[string]any{
			"start": "1s", "duration": "3s", "shape": "rounded_rect",
			"width": 720, "height": 96, "fill": "#22D3EE",
		}},
		{"text.add", map[string]any{
			"start": "4.5s", "duration": "3s", "text": "Desktop editor",
			"style": map[string]any{"size": 72, "color": "#FFFFFF"},
		}},
	}
	ops := make([]command.Operation, 0, len(specs))

	for _, spec := range specs {
		op, oerr := operation(spec.name, spec.args)
		if oerr != nil {
			return oerr
		}

		ops = append(ops, op)
	}

	if _, err = ui.sess.Edit(ctx, created.ProjectID, created.Revision, ops...); err != nil {
		return err
	}

	list, err := ui.sess.Projects(ctx)
	if err != nil {
		return err
	}

	ui.projects = list

	if rerr := ui.reloadSync(ctx, created.ProjectID); rerr != nil {
		return rerr
	}

	ui.playhead = 45
	ui.pxPerSec = 72

	for id := range ui.doc.Items {
		it := ui.doc.Items[id]
		if it.Text == "Vukem Video" {
			ui.selected = id
			ui.textEd.SetText(it.Text)
			ui.labelEd.SetText(it.Name)
			ui.boundSelection = id
		}
	}

	return nil
}

func (ui *UI) reloadSync(ctx context.Context, id string) error {
	got, err := ui.sess.Project(ctx, id)
	if err != nil {
		return err
	}

	media, err := ui.sess.Media(ctx, id)
	if err != nil {
		return err
	}

	ui.show(got, media)

	return nil
}

// HideProject keeps the project list and returns to the home screen.
func (ui *UI) HideProject() {
	ui.playing = false

	if ui.stage != nil {
		ui.stage.pause()
	}

	ui.projectID = ""
	ui.doc = nil
	ui.selected = ""
	ui.frame = frameView{}
}
