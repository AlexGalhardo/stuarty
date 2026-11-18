package gioapp

import (
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func (ui *UI) leftPanel(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(76))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X

			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return fill(gtx, colPanel)
				}),
				layout.Stacked(ui.tabs),
			)
		}),
		layout.Rigid(vline),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return fill(gtx, colBg)
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(10), Left: unit.Dp(10), Right: unit.Dp(10), Bottom: unit.Dp(10)}.
						Layout(gtx, ui.tabBody)
				}),
			)
		}),
	)
}

func (ui *UI) tabs(gtx layout.Context) layout.Dimensions {
	names := []struct {
		id  string
		tab tab
	}{
		{"Media", tabMedia},
		{"Text", tabText},
		{"Audio", tabAudio},
		{"Effects", tabEffects},
	}

	children := make([]layout.FlexChild, 0, len(names))
	for _, n := range names {
		children = append(children, layout.Rigid(ui.tabButton(n.id, n.tab)))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *UI) tabButton(name string, next tab) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		id := "tab-" + name
		if ui.clicked(gtx, id) {
			ui.tab = next
		}

		bg, fg := colPanel, colMuted
		if ui.tab == next {
			bg, fg = colAccentD, colAccent
		}

		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return button(gtx, ui.th, ui.click(id), name, bg, fg)
			})
	}
}

func (ui *UI) tabBody(gtx layout.Context) layout.Dimensions {
	switch ui.tab {
	case tabText:
		return ui.textTab(gtx)
	case tabAudio:
		return ui.assetTab(gtx, true)
	case tabEffects:
		return ui.effectsTab(gtx)
	default:
		return ui.assetTab(gtx, false)
	}
}

func (ui *UI) textTab(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, "Text")),
		gap(unit.Dp(8)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return editorBox(gtx, ui.th, &ui.draftEd, "Type a title")
		}),
		gap(unit.Dp(8)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "add-title") {
				ui.addText(96)
			}

			return button(gtx, ui.th, ui.click("add-title"), "Add title", colPanel2, colText)
		}),
		gap(unit.Dp(6)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "add-text") {
				ui.addText(48)
			}

			return button(gtx, ui.th, ui.click("add-text"), "Add text", colPanel2, colText)
		}),
		gap(unit.Dp(6)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "add-shape") {
				ui.addShape()
			}

			return button(gtx, ui.th, ui.click("add-shape"), "Add shape", colPanel2, colText)
		}),
	)
}

func (ui *UI) assetTab(gtx layout.Context, audio bool) layout.Dimensions {
	rows := ui.visibleAssets(audio)
	title := "Media"
	action := "add-media"

	if audio {
		title = "Audio"
		action = "add-audio"
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, lbl(ui.th, unit.Sp(12), colMuted, title)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.clicked(gtx, "import") {
						ui.openImport()
					}

					return button(gtx, ui.th, ui.click("import"), "Import", colPanel2, colText)
				}),
			)
		}),
		gap(unit.Dp(8)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			ui.assetList.Axis = layout.Vertical
			if len(rows) == 0 {
				return lbl(ui.th, unit.Sp(12), colMuted, "Nothing here yet.")(gtx)
			}

			return ui.assetList.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				a := rows[i]
				id := "asset-" + a.ID

				if ui.clicked(gtx, id) {
					ui.selectedAsset = a.ID
				}

				mark := a.Name
				if a.State != "" && a.State != "active" {
					mark += " (" + a.State + ")"
				}

				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return chip(gtx, ui.th, ui.click(id), ellipsize(mark, 28), ui.selectedAsset == a.ID)
				})
			})
		}),
		gap(unit.Dp(8)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, action) {
				ui.placeAsset(ui.selectedAsset, audio)
			}

			return button(gtx, ui.th, ui.click(action), "Add to timeline", colAccent, colBlack)
		}),
	)
}

func (ui *UI) visibleAssets(audio bool) []assetRow {
	out := make([]assetRow, 0, len(ui.assets))

	for _, a := range ui.assets {
		if a.State == "trashed" || a.Kind == string(timeline.AssetLUT) {
			continue
		}

		if audio {
			if a.Kind == string(timeline.AssetAudio) || a.HasAudio {
				out = append(out, a)
			}

			continue
		}

		if a.Kind == string(timeline.AssetVideo) || a.Kind == string(timeline.AssetImage) {
			out = append(out, a)
		}
	}

	return out
}

func (ui *UI) effectsTab(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, "Effects on the selection")),
		gap(unit.Dp(8)),
		layout.Rigid(ui.filterRow),
		gap(unit.Dp(8)),
		layout.Rigid(ui.speedRow),
		gap(unit.Dp(8)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "fade") {
				ui.fadeIn()
			}

			return button(gtx, ui.th, ui.click("fade"), "Fade in", colPanel2, colText)
		}),
		gap(unit.Dp(6)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "blur") {
				ui.setBlur(8)
			}

			return button(gtx, ui.th, ui.click("blur"), "Blur", colPanel2, colText)
		}),
	)
}

func (ui *UI) filterRow(gtx layout.Context) layout.Dimensions {
	return ui.choiceRow(gtx, []choice{
		{"fx-none", "None", func() { ui.setFilter("none") }},
		{"fx-gray", "Gray", func() { ui.setFilter("grayscale") }},
		{"fx-sepia", "Sepia", func() { ui.setFilter("sepia") }},
		{"fx-warm", "Warm", func() { ui.setFilter("warm") }},
		{"fx-cool", "Cool", func() { ui.setFilter("cool") }},
	})
}

func (ui *UI) speedRow(gtx layout.Context) layout.Dimensions {
	return ui.choiceRow(gtx, []choice{
		{"sp-050", "0.5x", func() { ui.setSpeed("0.5x") }},
		{"sp-100", "1x", func() { ui.setSpeed("1x") }},
		{"sp-150", "1.5x", func() { ui.setSpeed("1.5x") }},
		{"sp-200", "2x", func() { ui.setSpeed("2x") }},
	})
}

type choice struct {
	id, label string
	fn        func()
}

func (ui *UI) choiceRow(gtx layout.Context, items []choice) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(items)*2)

	for i, item := range items {
		if i > 0 {
			children = append(children, gap(unit.Dp(4)))
		}

		children = append(children, ui.choiceChip(item))
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (ui *UI) choiceChip(item choice) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if ui.clicked(gtx, item.id) {
			item.fn()
		}

		return chip(gtx, ui.th, ui.click(item.id), item.label, false)
	})
}

func (ui *UI) inspector(gtx layout.Context) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, colPanel)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, ui.inspectorBody)
		}),
	)
}

func (ui *UI) inspectorBody(gtx layout.Context) layout.Dimensions {
	it := ui.item(ui.selected)
	if it == nil {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(lbl(ui.th, unit.Sp(13), colText, "Inspector")),
			gap(unit.Dp(8)),
			layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, "Select a clip on the timeline.")),
		)
	}

	children := []layout.FlexChild{
		layout.Rigid(lbl(ui.th, unit.Sp(13), colText, "Inspector")),
		gap(unit.Dp(4)),
		layout.Rigid(
			lbl(ui.th, unit.Sp(12), colAccent, string(it.Type)+"  "+ellipsize(itemLabel(*it, ui.assetMap()), 24)),
		),
		gap(unit.Dp(4)),
		layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, ui.tc(it.Start)+"   "+ui.tc(it.Duration)+" long")),
		gap(unit.Dp(10)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return editorBox(gtx, ui.th, &ui.labelEd, "Name")
		}),
		gap(unit.Dp(6)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "apply-name") {
				ui.applyLabel()
			}

			return button(gtx, ui.th, ui.click("apply-name"), "Rename", colPanel2, colText)
		}),
	}

	if it.Type == timeline.ItemText || it.Type == timeline.ItemCaption {
		children = append(children,
			gap(unit.Dp(8)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return editorBox(gtx, ui.th, &ui.textEd, "Text")
			}),
			gap(unit.Dp(6)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if ui.clicked(gtx, "apply-text") {
					ui.applyText()
				}

				return button(gtx, ui.th, ui.click("apply-text"), "Update text", colPanel2, colText)
			}),
		)
	}

	if it.Type != timeline.ItemAudio {
		children = append(children,
			gap(unit.Dp(10)),
			layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, "Scale")),
			gap(unit.Dp(4)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.choiceRow(gtx, []choice{
					{"sc-050", "0.5", func() { ui.setTransform("scale", 0.5) }},
					{"sc-100", "1", func() { ui.setTransform("scale", 1) }},
					{"sc-150", "1.5", func() { ui.setTransform("scale", 1.5) }},
					{"sc-200", "2", func() { ui.setTransform("scale", 2) }},
				})
			}),
			gap(unit.Dp(8)),
			layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, "Opacity")),
			gap(unit.Dp(4)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.choiceRow(gtx, []choice{
					{"op-100", "100%", func() { ui.setTransform("opacity", 1) }},
					{"op-060", "60%", func() { ui.setTransform("opacity", 0.6) }},
					{"op-030", "30%", func() { ui.setTransform("opacity", 0.3) }},
				})
			}),
		)
	}

	if it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio {
		children = append(children,
			gap(unit.Dp(8)),
			layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, "Volume")),
			gap(unit.Dp(4)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.choiceRow(gtx, []choice{
					{"vol-0", "0", func() { ui.setVolume(0) }},
					{"vol-1", "1", func() { ui.setVolume(1) }},
					{"vol-2", "2", func() { ui.setVolume(2) }},
				})
			}),
		)
	}

	if it.Type == timeline.ItemClip {
		children = append(children,
			gap(unit.Dp(8)),
			layout.Rigid(lbl(ui.th, unit.Sp(12), colMuted, "Fit")),
			gap(unit.Dp(4)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.choiceRow(gtx, []choice{
					{"fit-cover", "Cover", func() { ui.setFit("cover") }},
					{"fit-contain", "Contain", func() { ui.setFit("contain") }},
				})
			}),
			gap(unit.Dp(8)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.choiceRow(gtx, []choice{
					{"br-0", "Neutral", func() { ui.setColor(0) }},
					{"br-up", "Brighter", func() { ui.setColor(0.15) }},
					{"br-dn", "Darker", func() { ui.setColor(-0.15) }},
				})
			}),
		)
	}

	children = append(children,
		gap(unit.Dp(12)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.clicked(gtx, "delete") {
				ui.deleteSelected()
			}

			return button(gtx, ui.th, ui.click("delete"), "Delete", colDanger, colWhite)
		}),
	)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *UI) assetMap() map[string]timeline.Asset {
	if ui.doc == nil {
		return nil
	}

	return ui.doc.Assets
}
