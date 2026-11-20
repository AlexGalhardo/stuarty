package image

import (
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
)

func init() {
	register(
		command.DefineIn(command.Spec{
			Name:    "layer.group",
			Summary: "Wraps layers that share one parent in a new group.",
			Description: "The group takes the place of the topmost of the layers and they keep their relative order inside " +
				"it. The group has an offset (x, y, 0 at first) added to its children and its opacity multiplies theirs.",
			Example: `{"layer_ids": ["lyr_a1b2c3d4e5", "lyr_f6g7h8j9k0"], "name": "Title block"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeInvalidState, command.CodeIDExists,
				command.CodeLimit,
			},
			Scope: command.ScopeEdit,
		}, applyGroup),
		command.DefineIn(command.Spec{
			Name:    "layer.ungroup",
			Summary: "Replaces a group by its children.",
			Description: "The children take the place of the group, their x and y gain the offset of the group and their " +
				"opacity is multiplied by the group's (rounded half up).",
			Example: `{"layer_id": "lyr_a1b2c3d4e5"}`,
			Errors: []string{
				command.CodeNotFound,
				command.CodeTypeMismatch,
				command.CodeInvalidState,
				command.CodeValidation,
			},
			Scope: command.ScopeEdit,
		}, applyUngroup),
		command.DefineIn(command.Spec{
			Name:    "layer.move",
			Summary: "Changes the order or the group of a layer.",
			Description: "parent is a group id, or root for the top level; index is the final position among the new " +
				"siblings, 0 is the bottom, and absent means on top. Without parent the layer stays in its group. A layer " +
				"cannot be moved into itself or into its own descendants.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "index": 0}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
				command.CodeLimit,
			},
			Scope: command.ScopeEdit,
		}, applyMove),
		command.DefineIn(command.Spec{
			Name:    "layer.duplicate",
			Summary: "Copies a layer with its whole subtree and new ids.",
			Description: "The copy sits just above the original, moved by dx and dy (default 0). A slot is not copied: a " +
				"slot name is unique in a document. effective.layer_id names the copy.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "dx": 40, "dy": 40}`,
			Errors:  []string{command.CodeValidation, command.CodeNotFound, command.CodeIDExists, command.CodeLimit},
			Scope:   command.ScopeEdit,
		}, applyDuplicate),
		command.DefineIn(command.Spec{
			Name:        "layer.remove",
			Summary:     "Removes a layer and everything inside it.",
			Description: "Undo puts the layer back. A group that holds a locked layer cannot be removed.",
			Example:     `{"layer_id": "lyr_a1b2c3d4e5"}`,
			Errors:      []string{command.CodeValidation, command.CodeNotFound, command.CodeInvalidState},
			Scope:       command.ScopeEdit,
		}, applyRemove),
	)
}

type groupArgs struct {
	LayerIDs []string `json:"layer_ids"      desc:"1 to 100 layers that share one parent." min:"1" max:"100" pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Name     string   `json:"name,omitempty"                                                       max:"120"`
	ID       string   `json:"id,omitempty"                                                                   pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyGroup(c *command.Context, d *Document, a *groupArgs) error {
	if err := limit("layers", Count(d), MaxLayers); err != nil {
		return err
	}

	var (
		parent  *Layer
		sibling *[]Layer
		indices []int
	)

	for i, id := range a.LayerIDs {
		path := "args.layer_ids[" + strconv.Itoa(i) + "]"

		if _, err := target(d, id, path); err != nil {
			return err
		}

		if slices.Contains(a.LayerIDs[:i], id) {
			return invalid(path, "Layer %s is listed twice.", id).WithHint("List each layer once.")
		}

		at, _ := locate(d, id)
		if i == 0 {
			parent, sibling = at.parent, at.siblings
		} else if at.siblings != sibling {
			return command.Errorf(command.CodeInvalidState, "Layer %s is not in the same group as %s.", id, a.LayerIDs[0]).
				WithHint("Group only layers that share one parent; move them together first with layer.move.").
				WithPath(path)
		}

		indices = append(indices, at.index)
	}

	depth := 0
	if parent != nil {
		depth = len(ancestors(d, parent.ID)) + 1
	}

	for _, i := range indices {
		if depth+1+height(&(*sibling)[i]) > MaxDepth {
			return limit("levels of groups", MaxDepth, MaxDepth)
		}
	}

	id, err := newLayerID(c, d, a.ID)
	if err != nil {
		return err
	}

	order := slices.Clone(indices)
	slices.Sort(order)

	top := order[len(order)-1]
	children := make([]Layer, 0, len(order))

	for _, i := range order {
		children = append(children, cloneLayer(&(*sibling)[i]))
	}

	for _, i := range slices.Backward(order) {
		*sibling = slices.Delete(*sibling, i, i+1)
	}

	g := base(id, KindGroup, a.Name, "Group")
	g.Children = children

	insert(sibling, top-(len(order)-1), g)
	reorder(*sibling)

	c.SetEffective("layer_id", id)

	return nil
}

type layerArgs struct {
	LayerID string `json:"layer_id" pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyUngroup(c *command.Context, d *Document, a *layerArgs) error {
	g, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	if g.Kind != KindGroup {
		return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; layer.ungroup changes groups.", g.ID, g.Kind).
			WithHint("Use layer.remove to remove a layer.").
			WithPath("args.layer_id")
	}

	at, _ := locate(d, g.ID)
	kids := cloneLayers(g.Children)

	for i := range kids {
		kids[i].X += g.X
		kids[i].Y += g.Y
		kids[i].Opacity = roundDiv(kids[i].Opacity*g.Opacity, 100)
		kids[i].Order = ""
	}

	if !g.Visible {
		for i := range kids {
			kids[i].Visible = false
		}
	}

	*at.siblings = slices.Replace(*at.siblings, at.index, at.index+1, kids...)
	reorder(*at.siblings)

	ids := make([]string, len(kids))
	for i := range kids {
		ids[i] = kids[i].ID
	}

	c.SetEffective("layer_ids", ids)

	return nil
}

type moveArgs struct {
	LayerID string `json:"layer_id"         pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Parent  string `json:"parent,omitempty"                                                        desc:"A group id, or root for the top level."`
	Index   *int   `json:"index,omitempty"                                                         desc:"Final position among the siblings, 0 is the bottom; absent means on top." min:"0" max:"500"`
}

func applyMove(c *command.Context, d *Document, a *moveArgs) error {
	if err := atLeastOne([]string{"parent", "index"}, a.Parent != "", a.Index != nil); err != nil {
		return err
	}

	l, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	at, _ := locate(d, l.ID)

	destID := ""
	if at.parent != nil {
		destID = at.parent.ID
	}

	if a.Parent != "" {
		destID = ""

		if a.Parent != "root" {
			g, gerr := target(d, a.Parent, "args.parent")
			if gerr != nil {
				return gerr
			}

			if g.Kind != KindGroup {
				return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; only a group has children.", g.ID, g.Kind).
					WithHint("Use a group, or root.").
					WithPath("args.parent")
			}

			if g.ID == l.ID || containsID(l, g.ID) {
				return command.Errorf(command.CodeInvalidState, "Layer %s cannot move into itself or its own children.", l.ID).
					WithHint("Choose a group outside the layer.").
					WithPath("args.parent")
			}

			destID = g.ID
		}
	}

	depth := 0
	if destID != "" {
		depth = len(ancestors(d, destID)) + 1
	}

	if depth+height(l) > MaxDepth {
		return limit("levels of groups", MaxDepth, MaxDepth)
	}

	// removing the layer shifts its siblings, so the destination is found again by the id of its group
	moved := removeAt(at)
	moved.Order = ""
	dest := &d.Layers

	if destID != "" {
		g, _ := Find(d, destID)
		dest = &g.Children
	}

	idx := -1
	if a.Index != nil {
		idx = *a.Index
	}

	insert(dest, idx, moved)
	reorder(*dest)

	c.SetEffective("layer_id", moved.ID)

	return nil
}

func containsID(l *Layer, id string) bool {
	for i := range l.Children {
		if l.Children[i].ID == id || containsID(&l.Children[i], id) {
			return true
		}
	}

	return false
}

type duplicateArgs struct {
	LayerID string `json:"layer_id"       pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	ID      string `json:"id,omitempty"   pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Name    string `json:"name,omitempty"                                                        max:"120"`
	DX      *int   `json:"dx,omitempty"                                                          max:"16384" min:"-16384"`
	DY      *int   `json:"dy,omitempty"                                                          max:"16384" min:"-16384"`
}

func applyDuplicate(c *command.Context, d *Document, a *duplicateArgs) error {
	l, err := reference(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	n := countSubtree(l)
	if Count(d)+n > MaxLayers {
		return limit("layers", MaxLayers, MaxLayers)
	}

	copied := cloneLayer(l)
	if _, err := regenerate(c, d, &copied, a.ID); err != nil {
		return err
	}

	copied.Order = ""
	copied.X += orElse(a.DX, 0)
	copied.Y += orElse(a.DY, 0)

	if a.Name != "" {
		copied.Name = a.Name
	} else if !strings.HasSuffix(copied.Name, " copy") {
		copied.Name = truncate(copied.Name+" copy", MaxName)
	}

	at, _ := locate(d, l.ID)
	insert(at.siblings, at.index+1, copied)
	reorder(*at.siblings)

	c.SetEffective("layer_id", copied.ID)

	return nil
}

// regenerate gives a subtree new ids in plan order (the first is the caller's when it supplied one) and drops
// its slots. The id of a copy must not be one the document has.
func regenerate(c *command.Context, d *Document, l *Layer, supplied string) (string, *command.Error) {
	first := true

	var visit func(l *Layer) *command.Error

	visit = func(l *Layer) *command.Error {
		sup := ""
		if first {
			sup, first = supplied, false
		}

		id, err := newLayerID(c, d, sup)
		if err != nil {
			return err
		}

		l.ID, l.Slot = id, ""

		for i := range l.Children {
			l.Children[i].Order = ""
			if err := visit(&l.Children[i]); err != nil {
				return err
			}
		}

		reorder(l.Children)

		return nil
	}

	if err := visit(l); err != nil {
		return "", err
	}

	return l.ID, nil
}

func countSubtree(l *Layer) int {
	n := 1
	for i := range l.Children {
		n += countSubtree(&l.Children[i])
	}

	return n
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n])
}

func applyRemove(c *command.Context, d *Document, a *layerArgs) error {
	l, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	if hasLocked(l) {
		return command.Errorf(command.CodeInvalidState, "Layer %s holds a locked layer.", l.ID).
			WithHint("Unlock the layers inside with layer.set first.").WithPath("args.layer_id")
	}

	gone := map[string]bool{}
	collectIDs(l, gone)

	at, _ := locate(d, l.ID)
	removeAt(at)
	pruneAssets(d)
	dropMasksFrom(d, gone)

	c.SetEffective("layer_id", a.LayerID)

	return nil
}

func hasLocked(l *Layer) bool {
	for i := range l.Children {
		if l.Children[i].Locked || hasLocked(&l.Children[i]) {
			return true
		}
	}

	return false
}
