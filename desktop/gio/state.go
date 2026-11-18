package gioapp

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeval"
	"github.com/AlexGalhardo/vukem-video/service"
)

type tab int

const (
	tabMedia tab = iota
	tabText
	tabAudio
	tabEffects
)

type assetRow struct {
	ID       string
	Name     string
	Kind     string
	State    string
	HasAudio bool
}

type frameView struct {
	busy bool
	want int
	have int
	img  image.Image
	err  string
}

type dragState struct {
	kind        hitKind
	item        string
	originStart int
	originDur   int
	originFrame int
	delta       int
	active      bool
}

// UI is the desktop editor. Fields are touched on the window goroutine, except
// through post, which the frame handler drains.
type UI struct {
	sess *Session
	th   *material.Theme
	wake func()

	mu      sync.Mutex
	pending []func()

	status   string
	fatal    string
	inflight bool

	projects  []service.ProjectSummary
	projectID string
	revision  int64
	doc       *timeline.Document
	assets    []assetRow

	tab           tab
	preset        string
	selected      string
	selectedAsset string
	playhead      int
	playing       bool
	last          time.Time
	pxPerSec      float64
	scrollX       int
	scrollY       int

	frame frameView

	importOpen     bool
	importDir      string
	importEntries  []fileEntry
	importSel      string
	exportOpen     bool
	format         string
	jobID          string
	jobBusy        bool
	jobPoll        time.Time
	drag           dragState
	boundSelection string

	stage       *stage
	configPath  string
	light       bool
	pal         palette
	seenSeq     int
	stageWasOn  bool
	audioNote   string
	clicks      map[string]*widget.Clickable
	nameEd      widget.Editor
	textEd      widget.Editor
	draftEd     widget.Editor
	labelEd     widget.Editor
	assetList   layout.List
	projectList layout.List
	fileList    layout.List

	timeTag    struct{}
	scrubTag   struct{}
	overlayTag struct{}
}

// New builds the editor. sess may be nil when the workspace could not be opened;
// fail carries that error.
func New(sess *Session, fail error) *UI {
	ui := &UI{
		sess:     sess,
		preset:   "16:9",
		pxPerSec: 80,
		format:   "mp4",
		clicks:   map[string]*widget.Clickable{},
	}
	ui.loadTheme()
	ui.th = newMaterial(ui.pal)
	ui.nameEd.SingleLine = true
	ui.textEd.SingleLine = true
	ui.draftEd.SingleLine = true
	ui.labelEd.SingleLine = true
	ui.nameEd.SetText("Untitled")
	ui.assetList.Axis = layout.Vertical
	ui.projectList.Axis = layout.Vertical
	ui.fileList.Axis = layout.Vertical

	if sess != nil {
		ui.stage = newStage(sess, nil)
	}

	if fail != nil {
		ui.fatal = errText(fail)
	}

	return ui
}

// SetWake is called when a background result is ready and the window should redraw.
func (ui *UI) SetWake(fn func()) {
	ui.wake = fn

	if ui.stage != nil {
		ui.stage.wake = fn
	}
}

func (ui *UI) post(fn func()) {
	ui.mu.Lock()
	ui.pending = append(ui.pending, fn)
	ui.mu.Unlock()

	if ui.wake != nil {
		ui.wake()
	}
}

func (ui *UI) drain() {
	ui.mu.Lock()
	fns := ui.pending
	ui.pending = nil
	ui.mu.Unlock()

	for _, fn := range fns {
		fn()
	}
}

func (ui *UI) async(status string, fn func(context.Context) error) {
	if ui.sess == nil || ui.inflight {
		return
	}

	ui.inflight = true
	ui.status = status

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		err := fn(ctx)
		ui.post(func() {
			ui.inflight = false

			if err != nil {
				ui.status = errText(err)

				return
			}

			if ui.status == status {
				ui.status = ""
			}
		})
	}()
}

// Boot loads the project list.
func (ui *UI) Boot() {
	if ui.sess == nil {
		return
	}

	ui.async("Loading projects…", func(ctx context.Context) error {
		list, err := ui.sess.Projects(ctx)
		if err != nil {
			return err
		}

		ui.post(func() { ui.projects = list })

		return nil
	})
}

func (ui *UI) click(id string) *widget.Clickable {
	c, ok := ui.clicks[id]
	if !ok {
		c = new(widget.Clickable)
		ui.clicks[id] = c
	}

	return c
}

func (ui *UI) clicked(gtx layout.Context, id string) bool {
	return ui.click(id).Clicked(gtx)
}

func (ui *UI) show(got *service.ProjectGetResult, media []service.MediaEntry) {
	if got == nil || got.Document == nil {
		ui.status = "The project has no document."

		return
	}

	ui.projectID = got.ProjectID
	ui.revision = got.Revision
	ui.doc = got.Document
	ui.assets = combineAssets(got.Document, media)

	if _, ok := ui.doc.Items[ui.selected]; !ok {
		ui.selected = ""
	}

	if ui.playhead > ui.endFrame() {
		ui.playhead = ui.endFrame()
	}

	ui.requestFrame()
}

func (ui *UI) reload(ctx context.Context, id string) error {
	got, err := ui.sess.Project(ctx, id)
	if err != nil {
		return err
	}

	media, err := ui.sess.Media(ctx, id)
	if err != nil {
		return err
	}

	ui.post(func() {
		ui.show(got, media)
		ui.requestFrame()
	})

	return nil
}

func (ui *UI) requestFrame() {
	if ui.sess == nil || ui.doc == nil || ui.projectID == "" {
		return
	}

	if ui.stage != nil && !ui.stage.failed {
		ui.stage.show(ui.doc, ui.revision, ui.playhead)

		return
	}

	ui.frame.want = ui.playhead
	if ui.frame.busy {
		return
	}

	ui.frame.busy = true
	ui.pumpFrame()
}

func (ui *UI) pumpFrame() {
	id := ui.projectID
	at := ui.frame.want
	rev := ui.revision

	go func() {
		raw, err := ui.sess.Frame(context.Background(), id, at)
		var img image.Image
		msg := ""

		if err != nil {
			msg = errText(err)
		} else if decoded, derr := jpeg.Decode(bytes.NewReader(raw)); derr != nil {
			msg = derr.Error()
		} else {
			img = decoded
		}

		ui.post(func() {
			ui.frame.busy = false

			if ui.projectID != id || ui.revision != rev {
				ui.requestFrame()

				return
			}

			if img != nil {
				ui.frame.img = img
				ui.frame.have = at
				ui.frame.err = ""
			} else if msg != "" {
				ui.frame.err = msg
			}

			if ui.frame.want != at {
				ui.frame.busy = true
				ui.pumpFrame()
			}
		})
	}()
}

// SyncFrame renders the current playhead on this goroutine, for a screenshot.
func (ui *UI) SyncFrame(ctx context.Context) {
	if ui.sess == nil || ui.doc == nil {
		return
	}

	raw, err := ui.sess.Frame(ctx, ui.projectID, ui.playhead)
	if err != nil {
		ui.frame.err = errText(err)

		return
	}

	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		ui.frame.err = err.Error()

		return
	}

	ui.frame.img = img
	ui.frame.have = ui.playhead
	ui.frame.err = ""
}

func combineAssets(doc *timeline.Document, media []service.MediaEntry) []assetRow {
	rows := make([]assetRow, 0, len(media)+len(doc.Assets))
	seen := map[string]bool{}

	for i := range media {
		m := media[i]
		if m.Trashed {
			continue
		}

		rows = append(rows, assetRow{
			ID: m.AssetID, Name: m.Name, Kind: m.Kind, State: m.State, HasAudio: m.HasAudio,
		})
		seen[m.AssetID] = true
	}

	for id := range doc.Assets {
		if seen[id] {
			continue
		}

		a := doc.Assets[id]
		rows = append(rows, assetRow{ID: id, Name: a.Name, Kind: string(a.Kind), State: "active", HasAudio: a.HasAudio})
	}

	return rows
}

func (ui *UI) item(id string) *timeline.Item {
	if ui.doc == nil || id == "" {
		return nil
	}

	it, ok := ui.doc.Items[id]
	if !ok {
		return nil
	}

	return &it
}

func (ui *UI) assetByID(id string) *assetRow {
	for i := range ui.assets {
		if ui.assets[i].ID == id {
			return &ui.assets[i]
		}
	}

	return nil
}

func (ui *UI) endFrame() int {
	end := 0

	if ui.doc != nil {
		for id := range ui.doc.Items {
			it := ui.doc.Items[id]
			if it.Start+it.Duration > end {
				end = it.Start + it.Duration
			}
		}
	}

	minFrame := int(ui.fps().Num)
	if minFrame < 1 {
		minFrame = 30
	}

	if end < minFrame {
		return minFrame
	}

	return end
}

func (ui *UI) fps() timeval.FPS {
	if ui.doc == nil || ui.doc.Settings.FPS.Num <= 0 || ui.doc.Settings.FPS.Den <= 0 {
		return timeval.FPS{Num: 30, Den: 1}
	}

	return timeval.FPS{Num: int64(ui.doc.Settings.FPS.Num), Den: int64(ui.doc.Settings.FPS.Den)}
}

func (ui *UI) tc(frame int) string {
	return ui.fps().Timecode(int64(frame))
}
