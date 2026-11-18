package gioapp

import (
	"path/filepath"
	"testing"
)

func TestThemeContrast(t *testing.T) {
	t.Parallel()

	for _, p := range []palette{darkPalette(), lightPalette()} {
		if contrast(p.text, p.panel) < 4.5 {
			t.Errorf("text on panel = %.2f", contrast(p.text, p.panel))
		}

		if contrast(p.text, p.bg) < 4.5 {
			t.Errorf("text on background = %.2f", contrast(p.text, p.bg))
		}

		if contrast(p.onAccent, p.accent) < 4.5 {
			t.Errorf("label on accent = %.2f", contrast(p.onAccent, p.accent))
		}

		if contrast(p.onClip, p.video) < 3 {
			t.Errorf("clip label = %.2f", contrast(p.onClip, p.video))
		}
	}
}

func TestThemePersists(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "vukem-video", "gio.json")
	ui := &UI{configPath: path}
	ui.useTheme(true, true)

	again := &UI{configPath: path}
	again.loadTheme()

	if !again.light || again.pal.bg != lightPalette().bg {
		t.Fatalf("reloaded light = %v bg = %#v", again.light, again.pal.bg)
	}

	again.useTheme(false, true)
	third := &UI{configPath: path}
	third.loadTheme()

	if third.light || third.pal.bg != darkPalette().bg {
		t.Fatalf("reloaded dark = %v", third.light)
	}
}
