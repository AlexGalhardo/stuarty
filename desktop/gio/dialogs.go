package gioapp

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/AlexGalhardo/vukem-video/service"
)

type fileEntry struct {
	name string
	path string
	dir  bool
}

func (ui *UI) overlay(gtx layout.Context) layout.Dimensions {
	if !ui.importOpen && !ui.exportOpen {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	size := gtx.Constraints.Max
	if size.X < gtx.Constraints.Min.X {
		size.X = gtx.Constraints.Min.X
	}

	if size.Y < gtx.Constraints.Min.Y {
		size.Y = gtx.Constraints.Min.Y
	}

	paint.FillShape(gtx.Ops, color.NRGBA{A: 160}, clip.Rect{Max: size}.Op())
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &ui.overlayTag)

	card := func(gtx layout.Context) layout.Dimensions {
		if ui.importOpen {
			return ui.importCard(gtx)
		}

		return ui.exportCard(gtx)
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = gtx.Dp(unit.Dp(560))
		gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(480))

		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				roundRect(gtx.Ops, ui.pal.panel, size, gtx.Dp(unit.Dp(8)))

				return layout.Dimensions{Size: size}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16), Bottom: unit.Dp(16), Left: unit.Dp(16), Right: unit.Dp(16)}.
					Layout(gtx, card)
			}),
		)
	})
}

func (ui *UI) openImport() {
	if ui.doc == nil {
		ui.status = "Open a project before importing."

		return
	}

	if ui.importDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			ui.importDir = home
		} else {
			ui.importDir = "."
		}
	}

	ui.readImportDir()
	ui.importOpen = true
	ui.exportOpen = false
}

func (ui *UI) readImportDir() {
	entries, err := os.ReadDir(ui.importDir)
	if err != nil {
		ui.status = err.Error()
		ui.importEntries = nil

		return
	}

	ui.importEntries = ui.importEntries[:0]

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		ui.importEntries = append(ui.importEntries, fileEntry{
			name: name, path: filepath.Join(ui.importDir, name), dir: e.IsDir(),
		})
	}

	slices.SortFunc(ui.importEntries, func(a, b fileEntry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}

			return 1
		}

		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
}

func (ui *UI) importCard(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(lbl(ui.th, unit.Sp(16), ui.pal.text, "Import media")),
		gap(unit.Dp(6)),
		layout.Rigid(lbl(ui.th, unit.Sp(12), ui.pal.muted, ellipsize(ui.importDir, 64))),
		gap(unit.Dp(8)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				ui.tool("import-up", "Up", ui.importUp),
				gap(unit.Dp(8)),
				ui.tool("import-close", "Close", func() { ui.importOpen = false }),
			)
		}),
		gap(unit.Dp(8)),
		layout.Flexed(1, ui.importList),
		gap(unit.Dp(8)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "import-do") {
				ui.importSelected()
			}

			return ui.button(gtx, ui.click("import-do"), "Import selected", ui.pal.accent, ui.pal.onAccent)
		}),
	)
}

func (ui *UI) importUp() {
	parent := filepath.Dir(ui.importDir)
	if parent == ui.importDir {
		return
	}

	ui.importDir = parent
	ui.importSel = ""
	ui.readImportDir()
}

func (ui *UI) importList(gtx layout.Context) layout.Dimensions {
	ui.fileList.Axis = layout.Vertical

	return ui.fileList.Layout(gtx, len(ui.importEntries), func(gtx layout.Context, i int) layout.Dimensions {
		e := ui.importEntries[i]
		id := "file-" + e.path
		label := e.name

		if e.dir {
			label += "/"
		}

		if ui.clicked(gtx, id) {
			if e.dir {
				ui.importDir = e.path
				ui.importSel = ""
				ui.readImportDir()
			} else {
				ui.importSel = e.path
			}
		}

		return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return ui.chip(gtx, ui.click(id), ellipsize(label, 48), ui.importSel == e.path)
		})
	})
}

func (ui *UI) importSelected() {
	if ui.importSel == "" {
		ui.status = "Select a file."

		return
	}

	path := ui.importSel
	id := ui.projectID
	ui.importOpen = false
	ui.async("Importing…", func(ctx context.Context) error {
		res, err := ui.sess.Import(ctx, id, []string{path})
		if err != nil {
			return err
		}

		if res.Failed > 0 {
			msg := "Import failed."
			if len(res.Assets) > 0 && res.Assets[0].Error != nil {
				msg = res.Assets[0].Error.Error()
			}

			ui.post(func() { ui.status = msg })
		}

		return ui.reload(ctx, id)
	})
}

func (ui *UI) exportCard(gtx layout.Context) layout.Dimensions {
	formats := []string{"mp4", "gif", "wav", "aac", "png"}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(lbl(ui.th, unit.Sp(16), ui.pal.text, "Export")),
		gap(unit.Dp(8)),
		layout.Rigid(lbl(ui.th, unit.Sp(12), ui.pal.muted, "The file is written into the workspace exports folder.")),
		gap(unit.Dp(12)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(formats)*2)

			for i, format := range formats {
				if i > 0 {
					children = append(children, gap(unit.Dp(6)))
				}

				children = append(children, ui.formatChip(format))
			}

			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
		}),
		gap(unit.Dp(16)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.clicked(gtx, "export-go") {
						ui.startExport()
					}

					return ui.button(gtx, ui.click("export-go"), "Start export", ui.pal.accent, ui.pal.onAccent)
				}),
				gap(unit.Dp(8)),
				ui.tool("export-close", "Close", func() { ui.exportOpen = false }),
			)
		}),
	)
}

func (ui *UI) formatChip(format string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ui.clicked(gtx, "fmt-"+format) {
			ui.format = format
		}

		return ui.chip(gtx, ui.click("fmt-"+format), format, ui.format == format)
	})
}

func (ui *UI) startExport() {
	if ui.doc == nil {
		return
	}

	id := ui.projectID
	format := ui.format
	at := ui.fps().FormatSeconds(int64(ui.playhead))
	ui.exportOpen = false
	ui.async("Starting export…", func(ctx context.Context) error {
		job, err := ui.sess.Export(ctx, id, format, at)
		if err != nil {
			return err
		}

		ui.post(func() {
			ui.jobID = job.JobID
			ui.applyJob(job, nil)
		})

		return nil
	})
}

func (ui *UI) pollJob(gtx layout.Context) {
	if ui.jobID == "" || ui.sess == nil {
		return
	}

	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second)})

	if ui.jobBusy {
		return
	}

	if !ui.jobPoll.IsZero() && gtx.Now.Sub(ui.jobPoll) < time.Second {
		return
	}

	ui.jobPoll = gtx.Now
	ui.jobBusy = true
	id := ui.jobID

	go func() {
		job, err := ui.sess.Job(context.Background(), id)
		ui.post(func() {
			ui.jobBusy = false
			ui.applyJob(job, err)
		})
	}()
}

func (ui *UI) applyJob(job *service.Job, err error) {
	if err != nil {
		ui.status = errText(err)
		ui.jobID = ""

		return
	}

	if job == nil {
		return
	}

	switch job.State {
	case service.JobSucceeded:
		ui.jobID = ""
		if job.Result != nil && job.Result.Export != nil && job.Result.Export.Path != "" {
			ui.status = "Exported " + job.Result.Export.Path

			return
		}

		ui.status = "Export finished."
	case service.JobFailed, service.JobCancelled:
		ui.jobID = ""
		if job.Error != nil {
			ui.status = job.Error.Error()

			return
		}

		ui.status = "Export failed."
	default:
		ui.status = fmt.Sprintf("Export %s %d%%", job.State, int(job.Progress*100))
	}
}
