package gioapp

import (
	"image"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/AlexGalhardo/vukem-video/web/app/agentdocs"
)

// Layout draws one frame of the editor.
func (ui *UI) Layout(gtx layout.Context) layout.Dimensions {
	ui.drain()
	ui.syncEditors()
	ui.keys(gtx)
	ui.tick(gtx)
	ui.pollJob(gtx)

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, ui.pal.bg)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(ui.topBar),
				layout.Rigid(ui.hline),
				layout.Rigid(ui.statusLine),
				layout.Flexed(1, ui.body),
			)
		}),
		layout.Expanded(ui.overlay),
	)
}

func (ui *UI) syncEditors() {
	if ui.boundSelection == ui.selected {
		return
	}

	ui.boundSelection = ui.selected
	it := ui.item(ui.selected)

	if it == nil {
		return
	}

	ui.textEd.SetText(it.Text)
	ui.labelEd.SetText(it.Name)
}

func (ui *UI) body(gtx layout.Context) layout.Dimensions {
	if ui.fatal != "" && ui.sess == nil {
		return ui.fatalScreen(gtx)
	}

	if ui.doc == nil {
		return ui.home(gtx)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(292)), gtx.Constraints.Max.Y))

					return ui.leftPanel(gtx)
				}),
				layout.Rigid(ui.vline),
				layout.Flexed(1, ui.preview),
				layout.Rigid(ui.vline),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(300)), gtx.Constraints.Max.Y))

					return ui.inspector(gtx)
				}),
			)
		}),
		layout.Rigid(ui.hline),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(248))))

			return ui.timeline(gtx)
		}),
	)
}

func (ui *UI) topBar(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(48))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, ui.pal.panel)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left:  unit.Dp(12),
				Right: unit.Dp(12),
			}.Layout(
				gtx,
				func(gtx layout.Context) layout.Dimensions {
					name := "No project"
					if ui.doc != nil {
						name = ui.doc.Name
					}

					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(lbl(ui.th, unit.Sp(14), ui.pal.accent, "Vukem")),
						gap(unit.Dp(8)),
						layout.Rigid(lbl(ui.th, unit.Sp(14), ui.pal.text, "Video")),
						gap(unit.Dp(16)),
						layout.Rigid(lbl(ui.th, unit.Sp(13), ui.pal.muted, ellipsize(name, 42))),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Min.Y)}
						}),
						layout.Rigid(lbl(ui.th, unit.Sp(12), ui.pal.muted, agentdocs.Version)),
						gap(unit.Dp(12)),
						ui.barButton("theme", ui.themeWord(), true, ui.toggleTheme),
						gap(unit.Dp(6)),
						ui.barButton("projects", "Projects", ui.doc != nil, ui.closeProject),
						gap(unit.Dp(6)),
						ui.barButton("undo", "Undo", ui.doc != nil, ui.undo),
						gap(unit.Dp(6)),
						ui.barButton("redo", "Redo", ui.doc != nil, ui.redo),
						gap(unit.Dp(10)),
						ui.barButton("export", "Export", ui.doc != nil, func() { ui.exportOpen = true }),
					)
				},
			)
		}),
	)
}

func (ui *UI) barButton(id, txt string, enabled bool, fn func()) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			return lbl(ui.th, unit.Sp(12), ui.pal.line, txt)(gtx)
		}

		bg, fg := ui.pal.panel2, ui.pal.text
		if id == "export" {
			bg, fg = ui.pal.accent, ui.pal.onAccent
		}

		if ui.clicked(gtx, "bar-"+id) {
			fn()
		}

		return ui.button(gtx, ui.click("bar-"+id), txt, bg, fg)
	})
}

func (ui *UI) statusLine(gtx layout.Context) layout.Dimensions {
	msg := ui.status
	if ui.fatal != "" {
		msg = ui.fatal
	}

	if msg == "" {
		return layout.Dimensions{}
	}

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx,
		lbl(ui.th, unit.Sp(12), ui.pal.muted, ellipsize(msg, 160)),
	)
}

func (ui *UI) fatalScreen(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(lbl(ui.th, unit.Sp(18), ui.pal.text, "Vukem Video could not open the workspace")),
			gap(unit.Dp(8)),
			layout.Rigid(lbl(ui.th, unit.Sp(13), ui.pal.muted, ellipsize(ui.fatal, 120))),
		)
	})
}

func (ui *UI) home(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Top:   unit.Dp(28),
				Left:  unit.Dp(36),
				Right: unit.Dp(24),
			}.Layout(
				gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(lbl(ui.th, unit.Sp(22), ui.pal.text, "Projects")),
						gap(unit.Dp(6)),
						layout.Rigid(lbl(ui.th, unit.Sp(13), ui.pal.muted, "Open a project or start a new one.")),
						gap(unit.Dp(16)),
						layout.Flexed(1, ui.projectListLayout),
					)
				},
			)
		}),
		layout.Rigid(ui.vline),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unit.Dp(360))
			gtx.Constraints = layout.Exact(image.Pt(w, gtx.Constraints.Max.Y))

			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return fill(gtx, ui.pal.panel)
				}),
				layout.Stacked(ui.homeForm),
			)
		}),
	)
}

func (ui *UI) projectListLayout(gtx layout.Context) layout.Dimensions {
	ui.projectList.Axis = layout.Vertical

	return ui.projectList.Layout(gtx, len(ui.projects), func(gtx layout.Context, i int) layout.Dimensions {
		p := ui.projects[i]
		id := p.ProjectID
		if id == "" {
			id = p.ID
		}

		if ui.clicked(gtx, "prj-"+id) {
			ui.openProject(id)
		}

		label := ellipsize(p.Name, 40)
		if size := summarySize(p); size != "" {
			label += "    " + size
		}

		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return ui.button(gtx, ui.click("prj-"+id), label, ui.pal.panel2, ui.pal.text)
		})
	})
}

func (ui *UI) keys(gtx layout.Context) {
	if ui.editorFocused(gtx) || ui.importOpen || ui.exportOpen {
		return
	}

	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameSpace},
			key.Filter{Name: key.NameDeleteBackward},
			key.Filter{Name: key.NameDeleteForward},
			key.Filter{Name: "S"},
			key.Filter{Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
		)
		if !ok {
			break
		}

		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}

		switch e.Name {
		case key.NameSpace:
			ui.togglePlay()
		case key.NameDeleteBackward, key.NameDeleteForward:
			ui.deleteSelected()
		case "S":
			ui.split()
		case "Z":
			if e.Modifiers.Contain(key.ModShift) {
				ui.redo()
			} else {
				ui.undo()
			}
		default:
		}
	}
}

func (ui *UI) editorFocused(gtx layout.Context) bool {
	for _, ed := range []*widget.Editor{&ui.nameEd, &ui.textEd, &ui.draftEd, &ui.labelEd} {
		if gtx.Focused(ed) {
			return true
		}
	}

	return false
}

func (ui *UI) togglePlay() {
	if ui.doc == nil {
		return
	}

	if ui.playing {
		ui.playing = false

		if ui.stage != nil {
			ui.stage.pause()
		}

		return
	}

	ui.playing = true
	ui.last = time.Time{}

	if ui.stage != nil && !ui.stage.failed {
		ui.stage.play(ui.doc, ui.revision, ui.playhead)
	}
}

func (ui *UI) takePreview() {
	if ui.stage == nil {
		return
	}

	snap := ui.stage.snapshot()
	if snap.seq > ui.seenSeq && snap.img != nil {
		ui.seenSeq = snap.seq
		ui.frame.img = snap.img
		ui.frame.have = snap.at
		ui.frame.err = ""

		if !snap.playing {
			ui.playhead = snap.at
		}
	}

	if snap.err != "" {
		ui.frame.err = snap.err
	}

	if snap.audio != "" && snap.audio != ui.audioNote {
		ui.audioNote = snap.audio
		ui.status = snap.audio
	}

	if snap.playing {
		ui.playing = true
		ui.playhead = snap.at
		ui.stageWasOn = true

		return
	}

	if ui.stageWasOn {
		ui.playing = false
		ui.playhead = snap.at
		ui.stageWasOn = false
	}
}

func (ui *UI) tick(gtx layout.Context) {
	live := ui.stage != nil && !ui.stage.failed
	if live {
		ui.takePreview()

		if ui.playing {
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 30)})
		}

		return
	}

	if !ui.playing || ui.doc == nil {
		ui.last = gtx.Now

		return
	}

	if ui.last.IsZero() {
		ui.last = gtx.Now
	}

	step := time.Second
	fps := ui.fps()

	if fps.Num > 0 {
		step = time.Duration(float64(time.Second) * float64(fps.Den) / float64(fps.Num))
	}

	if step <= 0 {
		step = time.Second / 30
	}

	advanced := 0

	for gtx.Now.Sub(ui.last) >= step && advanced < 8 {
		ui.last = ui.last.Add(step)
		ui.playhead++
		advanced++
	}

	if ui.playhead >= ui.endFrame() {
		ui.playhead = ui.endFrame()
		ui.playing = false
	}

	if advanced > 0 {
		ui.requestFrame()
	}

	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(step)})
}

func (ui *UI) seek(frame int) {
	if frame < 0 {
		frame = 0
	}

	if frame > ui.endFrame() {
		frame = ui.endFrame()
	}

	ui.playing = false

	if ui.stage != nil {
		ui.stage.pause()
	}

	ui.playhead = frame
	ui.requestFrame()
}
