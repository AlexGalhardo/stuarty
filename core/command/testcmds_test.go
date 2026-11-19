package command

import (
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	clipID    = "itm_m1n2p3q4r5"
	capItemID = "itm_s6t7v8w9x0"
	capTrack  = "trk_a1b2c3d4e5"
	vidTrack  = "trk_f6g7h8j9k0"
	assetID   = "ast_3c5v8n2w6d"
)

func readTimeline(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../timeline/testdata/valid/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func baseDoc(t *testing.T) *timeline.Document {
	t.Helper()
	doc, err := timeline.Decode(readTimeline(t, "launch-teaser"))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// Test commands: a small catalogue that touches every collection, so the machinery
// of Apply, the diff, the inverse and the primitives is tested without the real
// commands (SDD-002 Part C is task P1-ENG-002).

type tNameArgs struct {
	Name string `json:"name" min:"1" max:"120"`
}

type tTrackAddArgs struct {
	ID    string `json:"id,omitempty"`
	Kind  string `json:"kind"           enum:"visual,audio"`
	Order string `json:"order"`
	Name  string `json:"name,omitempty"`
}

type tItemAddArgs struct {
	TrackID  string `json:"track_id"`
	ID       string `json:"id,omitempty"`
	Start    string `json:"start"`
	Duration int    `json:"duration"       min:"1" max:"108000"`
	Text     string `json:"text,omitempty"`
}

type tItemMoveArgs struct {
	ItemID string `json:"item_id"`
	Start  string `json:"start"`
}

type tItemRemoveArgs struct {
	ItemIDs []string `json:"item_ids" min:"1" max:"200"`
}

type tSettingsArgs struct {
	Width  *int `json:"width,omitempty"`
	Height *int `json:"height,omitempty"`
}

type tAssetArgs struct {
	Asset map[string]any `json:"asset"`
}

type tFontArgs struct {
	Family string `json:"family"`
	Hash   string `json:"hash,omitempty"`
}

type noArgs struct{}

type panicArgs struct {
	Mode string `json:"mode" enum:"panic,plain,bug,typed"`
}

var errorsOfTest = []string{CodeValidation, CodeNotFound, CodeIDExists, CodeOverlap, CodeInvalidTime, CodeInternal}

func testCommands() []Command {
	return []Command{
		Define(Spec{
			Name: "test.name", Summary: "Renames the project.", Description: "Sets the project name.",
			Example: `{"name": "Teaser"}`, Errors: []string{CodeValidation}, Scope: ScopeEdit,
		}, func(c *Context, a *tNameArgs) error {
			c.Doc.Name = a.Name
			return nil
		}),
		Define(Spec{
			Name: "test.track_add", Summary: "Adds a track.", Description: "Adds an empty track with the given order.",
			Example: `{"kind": "visual", "order": "b0", "id": "trk_k000000001"}`,
			Errors:  []string{CodeValidation, CodeIDExists}, Scope: ScopeEdit,
		}, func(c *Context, a *tTrackAddArgs) error {
			id, err := c.NewID(timeline.PrefixTrack, a.ID)
			if err != nil {
				return err
			}
			if _, dup := c.Doc.Tracks[id]; dup {
				return Errorf(CodeIDExists, "Track %s already exists.", id).WithHint("Use another id or omit id.")
			}
			c.Doc.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackKind(a.Kind), Order: a.Order, Name: a.Name}
			c.SetEffective("track_id", id)
			return nil
		}),
		Define(Spec{
			Name: "test.item_add", Summary: "Adds a text item.", Description: "Adds a text item on a visual track.",
			Example: `{"track_id": "trk_a1b2c3d4e5", "start": "100f", "duration": 10}`,
			Errors:  errorsOfTest, Scope: ScopeEdit,
		}, func(c *Context, a *tItemAddArgs) error {
			if _, ok := c.Doc.Tracks[a.TrackID]; !ok {
				return Errorf(CodeNotFound, "Track %s does not exist.", a.TrackID).
					WithHint("Read the project for the track ids.").
					WithDetail("kind", "track").WithDetail("id", a.TrackID)
			}
			start, err := c.ParseTime("args.start", a.Start)
			if err != nil {
				return err
			}
			id, err := c.NewID(timeline.PrefixItem, a.ID)
			if err != nil {
				return err
			}
			it := timeline.NewItem(id, timeline.ItemText, a.TrackID, start, a.Duration)
			it.Text = a.Text
			if it.Text == "" {
				it.Text = "x"
			}
			if _, dup := c.Doc.Items[id]; dup {
				return Errorf(CodeIDExists, "Item %s already exists.", id).WithHint("Use another id or omit id.")
			}
			c.Doc.Items[id] = it
			c.SetEffectiveTime("start", start, a.Start)
			return nil
		}),
		Define(Spec{
			Name: "test.item_move", Summary: "Moves an item.", Description: "Sets the start of an item.",
			Example: `{"item_id": "itm_m1n2p3q4r5", "start": "30f"}`,
			Errors:  errorsOfTest, Scope: ScopeEdit,
		}, func(c *Context, a *tItemMoveArgs) error {
			it, ok := c.Doc.Items[a.ItemID]
			if !ok {
				return Errorf(CodeNotFound, "Item %s does not exist.", a.ItemID).
					WithHint("Read the project for the item ids.").
					WithDetail("kind", "item").WithDetail("id", a.ItemID).
					WithPath("args.item_id")
			}
			start, err := c.ParseTime("args.start", a.Start)
			if err != nil {
				return err
			}
			it.Start = start
			c.Doc.Items[a.ItemID] = it
			c.SetEffectiveTime("start", start, a.Start)
			return nil
		}),
		Define(Spec{
			Name: "test.item_remove", Summary: "Removes items.", Description: "Deletes the given items.",
			Example: `{"item_ids": ["itm_m1n2p3q4r5"]}`, Errors: []string{CodeValidation, CodeNotFound},
			Scope: ScopeEdit,
		}, func(c *Context, a *tItemRemoveArgs) error {
			for _, id := range a.ItemIDs {
				if _, ok := c.Doc.Items[id]; !ok {
					return Errorf(
						CodeNotFound,
						"Item %s does not exist.",
						id,
					).WithHint("Read the project for the item ids.")
				}
				delete(c.Doc.Items, id)
			}
			return nil
		}),
		Define(Spec{
			Name:        "test.settings",
			Summary:     "Sets the frame size.",
			Description: "Sets width and height.",
			Example:     `{"width": 1920, "height": 1080}`,
			Errors:      []string{CodeValidation},
			Scope:       ScopeEdit,
		}, func(c *Context, a *tSettingsArgs) error {
			if a.Width != nil {
				c.Doc.Settings.Width = *a.Width
			}
			if a.Height != nil {
				c.Doc.Settings.Height = *a.Height
			}
			return nil
		}),
		Define(Spec{
			Name: "test.asset_add", Summary: "Adds an asset.", Description: "Adds an asset object as given.",
			Example: `{"asset": {"id": "ast_k000000001"}}`, Errors: []string{CodeValidation}, Scope: ScopeMedia,
			Internal: true,
		}, func(c *Context, a *tAssetArgs) error {
			id, _ := a.Asset["id"].(string)
			c.Doc.Assets[id] = timeline.Asset{
				ID:     id,
				Hash:   c.Doc.Assets[assetID].Hash,
				Kind:   timeline.AssetImage,
				Name:   "added",
				Width:  10,
				Height: 10,
			}
			return nil
		}),
		Define(Spec{
			Name: "test.font", Summary: "Adds a font.", Description: "Adds or replaces a user font.",
			Example: `{"family": "Mine"}`, Errors: []string{CodeValidation}, Scope: ScopeEdit,
		}, func(c *Context, a *tFontArgs) error {
			if c.Doc.Fonts == nil {
				c.Doc.Fonts = map[string]timeline.ProjectFont{}
			}
			hash := a.Hash
			if hash == "" {
				hash = c.Doc.Assets[assetID].Hash
			}
			c.Doc.Fonts[a.Family] = timeline.ProjectFont{Files: []timeline.FontFile{{Weight: 400, Hash: hash}}}
			return nil
		}),
		Define(Spec{
			Name: "test.misbehave", Summary: "Fails in ways a command should not.", Description: "Test only.",
			Example: `{"mode": "bug"}`, Errors: []string{CodeInternal}, Scope: ScopeEdit,
		}, func(c *Context, a *panicArgs) error {
			switch a.Mode {
			case "panic":
				panic("secret C:\\server\\path.go:12")
			case "plain":
				return errors.New("select * from users at /var/db/file.go:3")
			case "typed":
				return Errorf(CodeNotFound, "Nothing to find.").WithHint("Look elsewhere.")
			}
			it := c.Doc.Items[clipID]
			it.Start = -5
			c.Doc.Items[clipID] = it
			return nil
		}),
		Define(Spec{
			Name: "test.noop", Summary: "Does nothing.", Description: "Changes nothing.",
			Example: `{}`, Errors: []string{CodeValidation}, Scope: ScopeEdit,
		}, func(*Context, *noArgs) error { return nil }),
	}
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	r.Debug = true
	cmds := testCommands()
	for i := range cmds {
		if err := r.Register(cmds[i]); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

type seqIDs struct{ n int }

func (s *seqIDs) next(prefix string) string {
	s.n++
	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	return prefix + "_n" + strings.Repeat("0", 7) + string(alphabet[s.n/32%32]) + string(alphabet[s.n%32])
}

func opJSON(op, args string) Operation {
	return Operation{Op: op, Args: []byte(args)}
}

// fullRegistry is every registered command, with validation of every
// result on: what the golden cases and the registry-wide gates run against.
func fullRegistry() *Registry {
	r := FullRegistry()
	r.Debug = true

	return r
}
