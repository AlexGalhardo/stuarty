package gioapp

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/AlexGalhardo/vukem-video/service"
)

func (ui *UI) homeForm(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(28), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(lbl(ui.th, unit.Sp(16), ui.pal.text, "New project")),
				gap(unit.Dp(12)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.editorBox(gtx, &ui.nameEd, "Project name")
				}),
				gap(unit.Dp(12)),
				layout.Rigid(ui.presetRow),
				gap(unit.Dp(16)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.clicked(gtx, "create") {
						ui.createProject()
					}

					return ui.button(gtx, ui.click("create"), "Create project", ui.pal.accent, ui.pal.onAccent)
				}),
				gap(unit.Dp(18)),
				layout.Rigid(lbl(ui.th, unit.Sp(12), ui.pal.muted, "16:9, 9:16, 1:1 and 4:5, at 30 fps.")),
			)
		})
}

func (ui *UI) presetRow(gtx layout.Context) layout.Dimensions {
	presets := []string{"16:9", "9:16", "1:1", "4:5"}
	children := make([]layout.FlexChild, 0, len(presets)*2)

	for i, preset := range presets {
		if i > 0 {
			children = append(children, gap(unit.Dp(6)))
		}

		children = append(children, ui.presetChip(preset))
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (ui *UI) presetChip(preset string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ui.clicked(gtx, "preset-"+preset) {
			ui.preset = preset
		}

		return ui.chip(gtx, ui.click("preset-"+preset), preset, ui.preset == preset)
	})
}

func summarySize(p service.ProjectSummary) string {
	if p.Width == 0 || p.Height == 0 {
		return ""
	}

	return fmt.Sprintf("%d×%d", p.Width, p.Height)
}
