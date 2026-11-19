package image

import (
	"strings"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// AssetInfo is what the caller knows of an image file: the facts the core needs and cannot read.
type AssetInfo struct {
	Name   string
	Width  int
	Height int
}

// Env is what a batch needs from outside, so that the core stays pure: the same document, operations and
// environment always give the same result.
type Env struct {
	// NewIDs makes an identifier for a prefix; a command uses it only when the caller supplied no id.
	NewIDs func(prefix string) string
	// Asset says whether a content hash is a ready image and gives its size. Without it every layer.add_image
	// is refused with E_NOT_FOUND.
	Asset func(hash string) (AssetInfo, bool)
	// Measure gives the natural size of a text as the rasterizer lays it out. Without it the height of a text is
	// estimated from its lines and its size.
	Measure plan.Measurer
}

func (e Env) command() command.Env { return command.Env{NewIDs: e.NewIDs, Extra: &e} }

func envOf(c *command.Context) *Env {
	if e, ok := c.Env.Extra.(*Env); ok {
		return e
	}

	return &Env{NewIDs: c.Env.NewIDs}
}

var (
	registryOnce sync.Once
	registry     *command.Registry
	catalogue    []command.Command
)

// register adds a command to the catalogue; the command files call it from init.
func register(cs ...command.Command) { catalogue = append(catalogue, cs...) }

// Registry is the registry of the commands of domain image (SDD-021 section 3).
func Registry() *command.Registry {
	registryOnce.Do(func() {
		registry = command.NewRegistry()
		registry.MustRegister(catalogue...)
	})

	return registry
}

func idForm(path, id string) *command.Error {
	if id != "" && !timeline.ValidID(PrefixLayer, id) {
		return command.Errorf(command.CodeValidation, "%s %q is not a layer id.", path, id).
			WithHint("A layer id is lyr_ and 10 characters from 0-9 and a-z, without i, l, o and u.").WithPath(path)
	}

	return nil
}

func invalid(path, format string, a ...any) *command.Error {
	return command.Errorf(command.CodeValidation, format, a...).WithPath(path)
}

// layerNotFound is E_NOT_FOUND for a layer id.
func layerNotFound(id, path string) *command.Error {
	return command.Errorf(command.CodeNotFound, "Layer %s does not exist.", id).
		WithHint("Use the id of a layer listed by the description of the image.").
		WithDetail("kind", "layer").WithDetail("id", id).WithPath(path)
}

// target finds a layer that a command is going to change: it must exist and not be locked.
func target(d *Document, id, path string) (*Layer, *command.Error) {
	if err := idForm(path, id); err != nil {
		return nil, err
	}

	l, ok := Find(d, id)
	if !ok {
		return nil, layerNotFound(id, path)
	}

	if effectivelyLocked(d, id) {
		return nil, command.Errorf(command.CodeInvalidState, "Layer %s is locked.", id).
			WithHint("Unlock it with layer.set (locked false), then change it.").WithPath(path)
	}

	return l, nil
}

// reference finds a layer that a command only reads.
func reference(d *Document, id, path string) (*Layer, *command.Error) {
	if err := idForm(path, id); err != nil {
		return nil, err
	}

	l, ok := Find(d, id)
	if !ok {
		return nil, layerNotFound(id, path)
	}

	return l, nil
}

// colour checks a colour argument and returns its stored form, upper case.
func colour(path, v string) (string, *command.Error) {
	up := strings.ToUpper(v)
	if !ValidColour(up) {
		return "", invalid(path, "%s %q is not a colour.", path, v).WithHint(`Send "#RRGGBB" or "#RRGGBBAA".`)
	}

	return up, nil
}

// atLeastOne is E_VALIDATION when none of the optional arguments was given.
func atLeastOne(names []string, given ...bool) *command.Error {
	for _, g := range given {
		if g {
			return nil
		}
	}

	return invalid("args", "Give at least one of: %s.", strings.Join(names, ", ")).
		WithHint("Add one of %s to the arguments.", strings.Join(names, ", "))
}

func limit(what string, have, ceiling int) *command.Error {
	if have < ceiling {
		return nil
	}

	return command.Errorf(command.CodeLimit, "The document already has %d %s; the limit is %d.", have, what, ceiling).
		WithHint("Remove %s that you no longer need, or put the work in another image.", what).
		WithDetail("limit", what).WithDetail("value", have).WithDetail("ceiling", ceiling)
}

// newLayerID is the id of a new layer: the caller's, which must be free, or a generated one.
func newLayerID(c *command.Context, d *Document, supplied string) (string, *command.Error) {
	const path = "args.id"

	if err := idForm(path, supplied); err != nil {
		return "", err
	}

	id, err := c.NewID(PrefixLayer, supplied)
	if err != nil {
		return "", err
	}

	if _, taken := Find(d, id); taken {
		return "", command.Errorf(command.CodeIDExists, "Layer %s already exists.", id).
			WithHint("Omit id, and a new one is made.").WithPath(path)
	}

	return id, nil
}

// slotName checks a slot name and that no other layer uses it.
func slotName(d *Document, name, self, path string) *command.Error {
	if !slotPattern.MatchString(name) {
		return invalid(path, "%s %q is not a slot name.", path, name).
			WithHint("A slot name is a lower case letter followed by up to 39 letters, digits or underscores.")
	}

	for _, l := range Slots(d) {
		if l.Slot == name && l.ID != self {
			return command.Errorf(command.CodeIDExists, "The slot %q is already used by layer %s.", name, l.ID).
				WithHint("Slot names are unique in a document.").WithPath(path)
		}
	}

	return nil
}

// place puts a new layer into the document at a parent (a group id, or "" for the top level) and an index.
func place(d *Document, l Layer, parent string, index *int) *command.Error {
	const pathParent = "args.parent"

	if err := limit("layers", Count(d), MaxLayers); err != nil {
		return err
	}

	siblings := &d.Layers
	depth := 0

	if parent != "" {
		g, err := target(d, parent, pathParent)
		if err != nil {
			return err
		}

		if g.Kind != KindGroup {
			return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; only a group has children.", parent, g.Kind).
				WithHint("Use a group, or add the layer at the top level.").
				WithPath(pathParent)
		}

		siblings = &g.Children
		depth = len(ancestors(d, parent)) + 1
	}

	if depth+height(&l) > MaxDepth {
		return limit("levels of groups", MaxDepth, MaxDepth)
	}

	at := -1
	if index != nil {
		at = *index
	}

	insert(siblings, at, l)
	reorder(*siblings)

	return nil
}

// round is a whole number from a ratio, half up: the one rounding rule of the document.
func roundDiv(num, den int) int {
	if den < 0 {
		num, den = -num, -den
	}

	return floorDiv(2*num+den, 2*den)
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}

	return q
}
