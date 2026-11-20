package image

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// maxNameRunes is the longest name of a document or of a layer (the `max` of image.create and layer.add_image).
const maxNameRunes = 120

// ImportSource is what the service knows of the file an image import reads: its content hash, its size in pixels
// and the name it is known by.
type ImportSource struct {
	Hash          string
	Width, Height int
	Name          string
}

// ImportCanvas is what the caller says about the canvas of a new document: a preset, or both sides. All empty
// means "the size of the source".
type ImportCanvas struct {
	Preset        string
	Width, Height int
}

// ImportName is the name of a document made from a file: the name of the file without its directory and its
// extension, at most 120 characters, Untitled when nothing is left.
func ImportName(file string) string {
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		file = file[i+1:]
	}

	if i := strings.LastIndex(file, "."); i > 0 {
		file = file[:i]
	}

	file = strings.TrimSpace(file)
	if file == "" {
		return "Untitled"
	}

	return clipName(file)
}

// clipName cuts a name to the 120 characters a layer or a document may have.
func clipName(name string) string {
	if utf8.RuneCountInString(name) > maxNameRunes {
		return string([]rune(name)[:maxNameRunes])
	}

	return name
}

// ImportBatch is the batch that makes a new document from an image file (SDD-021 section 10.1): image.create,
// then layer.add_image with the fit contain and no box, which centres the whole source on the canvas. The canvas
// is the size of the source when both sides fit the canvas limit and nothing else was asked; a larger source
// needs a preset or both sides (E_LIMIT_EXCEEDED, with a hint). name is the name of the document; empty means
// the name of the file.
func ImportBatch(src ImportSource, canvas ImportCanvas, name string) ([]command.Operation, error) {
	create := map[string]any{}

	switch {
	case canvas.Preset != "" && (canvas.Width != 0 || canvas.Height != 0):
		return nil, invalid("preset", "Give a preset or a width and a height, not both.").
			WithHint("Remove width and height, or remove preset.")
	case canvas.Preset != "":
		if _, ok := PresetByName(canvas.Preset); !ok {
			return nil, invalid(
				"preset",
				"The preset %q is not one of %s.",
				canvas.Preset,
				strings.Join(PresetNames(), ", "),
			)
		}

		create["preset"] = canvas.Preset
	case canvas.Width != 0 || canvas.Height != 0:
		if canvas.Width < 1 || canvas.Height < 1 || canvas.Width > MaxCanvas || canvas.Height > MaxCanvas {
			return nil, invalid("width", "The canvas is %dx%d; give both sides, each 1 to %d.",
				canvas.Width, canvas.Height, MaxCanvas)
		}

		create["width"], create["height"] = canvas.Width, canvas.Height
	case src.Width > MaxCanvas || src.Height > MaxCanvas:
		return nil, command.Errorf(command.CodeLimit,
			"The image is %dx%d; a canvas is at most %d pixels on a side.", src.Width, src.Height, MaxCanvas).
			WithHint("Give width and height (each 1 to %d) or a preset (%s) for the canvas.",
				MaxCanvas, strings.Join(PresetNames(), ", ")).
			WithDetail("limit", "canvas").WithDetail("value", max(src.Width, src.Height)).
			WithDetail("ceiling", MaxCanvas)
	case src.Width < 1 || src.Height < 1:
		return nil, invalid("hash", "The file has no size.")
	default:
		create["width"], create["height"] = src.Width, src.Height
	}

	if name == "" {
		name = ImportName(src.Name)
	}

	create["name"] = clipName(name)

	add := map[string]any{"asset": src.Hash, "fit": "contain"}

	createArgs, err := json.Marshal(create)
	if err != nil {
		return nil, command.Errorf(command.CodeInternal, "The import batch could not be built.")
	}

	addArgs, err := json.Marshal(add)
	if err != nil {
		return nil, command.Errorf(command.CodeInternal, "The import batch could not be built.")
	}

	return []command.Operation{
		{Op: "image.create", Args: createArgs},
		{Op: "layer.add_image", Args: addArgs},
	}, nil
}

// ImportLayer is the operation that puts a file in an existing document as one more layer: fit contain, centred.
// name is the name of the layer; empty means the name the file is known by.
func ImportLayer(hash, name string) (command.Operation, error) {
	args := map[string]any{"asset": hash, "fit": "contain"}
	if name != "" {
		args["name"] = clipName(name)
	}

	raw, err := json.Marshal(args)
	if err != nil {
		return command.Operation{}, command.Errorf(command.CodeInternal, "The import operation could not be built.")
	}

	return command.Operation{Op: "layer.add_image", Args: raw}, nil
}
