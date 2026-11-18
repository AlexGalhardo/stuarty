// Command vukem-video-desktop is the Gio editor (ADR-0030).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	gioapp "github.com/AlexGalhardo/vukem-video/desktop/gio"
	"github.com/AlexGalhardo/vukem-video/web/app/agentdocs"
)

type options struct {
	workspace string
	demo      bool
	shot      string
	home      string
	gallery   string
	limit     time.Duration
}

func main() {
	opt := parseOptions()
	dir, temp, err := pickDir(opt)
	if err != nil {
		exit(err)
	}

	sess, openErr := gioapp.Open(dir)
	ui := gioapp.New(sess, openErr)

	if opt.gallery != "" {
		if err := runGallery(ui, sess, opt.gallery); err != nil {
			exit(err)
		}

		return
	}

	if opt.shot != "" || opt.home != "" || opt.demo {
		if err := writeShots(ui, sess, dir, temp, opt); err != nil {
			exit(err)
		}

		if opt.shot != "" || opt.home != "" {
			if !opt.demo {
				return
			}
		}
	} else if openErr == nil {
		ui.Boot()
	}

	runWindow(ui, sess, opt.limit)
}

func parseOptions() options {
	workspace := flag.String("workspace", "", "workspace directory (default: VUKEM_VIDEO_HOME or the user default)")
	demo := flag.Bool("demo", false, "create a sample project and open it")
	shot := flag.String("screenshot", "", "write a PNG of the editor and exit")
	home := flag.String("screenshot-home", "", "write a PNG of the project list and exit")
	gallery := flag.String("gallery", "", "write the editor screenshots into this directory and exit")
	seconds := flag.Int("seconds", 0, "close the window after this many seconds (0 keeps it open)")
	flag.Parse()

	return options{
		workspace: *workspace, demo: *demo, shot: *shot, home: *home, gallery: *gallery,
		limit: time.Duration(*seconds) * time.Second,
	}
}

func pickDir(opt options) (string, bool, error) {
	if (opt.shot != "" || opt.home != "" || opt.gallery != "") && opt.workspace == "" {
		dir, err := os.MkdirTemp("", "vukem-gio-")

		return dir, true, err
	}

	return opt.workspace, false, nil
}

func writeShots(ui *gioapp.UI, sess *gioapp.Session, dir string, temp bool, opt options) error {
	if sess == nil {
		return errOpen
	}

	ctx := context.Background()
	if err := ui.SeedDemo(ctx); err != nil {
		return err
	}

	if opt.shot != "" {
		ui.SyncFrame(ctx)
		if err := gioapp.RenderPNG(ui, opt.shot, 1600, 900); err != nil {
			return err
		}
	}

	if opt.home != "" {
		ui.HideProject()
		if err := gioapp.RenderPNG(ui, opt.home, 1600, 900); err != nil {
			return err
		}
	}

	if (opt.shot != "" || opt.home != "") && !opt.demo {
		_ = sess.Close()
		if temp {
			_ = os.RemoveAll(dir)
		}
	}

	return nil
}

func runGallery(ui *gioapp.UI, sess *gioapp.Session, dir string) error {
	if sess == nil {
		return errOpen
	}

	defer ui.ClosePreview()
	defer func() { _ = sess.Close() }()

	return ui.Gallery(dir)
}

var errOpen = errors.New("the workspace is not open")

func runWindow(ui *gioapp.UI, sess *gioapp.Session, limit time.Duration) {
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("Vukem Video"),
			app.Size(unit.Dp(1440), unit.Dp(860)),
			app.MinSize(unit.Dp(1100), unit.Dp(680)),
		)
		ui.SetWake(w.Invalidate)

		err := loop(w, ui, limit)
		ui.ClosePreview()

		if sess != nil {
			_ = sess.Close()
		}

		if err != nil {
			fmt.Fprintln(os.Stderr, "vukem-video-desktop:", err)
			os.Exit(1)
		}

		os.Exit(0)
	}()

	app.Main()
}

func loop(w *app.Window, ui *gioapp.UI, limit time.Duration) error {
	started := time.Now()
	if limit > 0 {
		// A quiet window does not ask for another frame, so the deadline is
		// checked on a frame that this timer requests.
		go func() {
			time.Sleep(limit)
			w.Invalidate()
		}()
	}

	var ops op.Ops

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			if limit > 0 && time.Since(started) > limit {
				w.Perform(system.ActionClose)
			}

			gtx := app.NewContext(&ops, e)
			ui.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, "vukem-video-desktop:", err)
	os.Exit(1)
}

// The desktop binary reports the same version as the server.
var _ = agentdocs.Version
