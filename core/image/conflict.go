package image

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Conflicts implements SDD-002 section A.7 step 2 for an image: it reports the first object that ops name in
// their arguments, in the order of the operations and of the arguments, that any of the changes committed in
// the meantime added, updated or removed. The unit is the object, not the property (ADR-0006): a layer, or the
// document itself for an operation that sets the canvas.
func Conflicts(ops []command.Operation, intervening []command.Change) *command.ConflictInfo {
	changed := map[string]command.Change{}

	for _, ch := range intervening {
		prefix := kindPrefix(ch.Kind)
		if prefix != "layer" && prefix != "document" {
			continue
		}

		key := prefix + " " + ch.ID
		if _, seen := changed[key]; !seen {
			changed[key] = ch
		}
	}

	for _, op := range ops {
		if op.Op == "image.create" || op.Op == "canvas.resize" {
			for _, ch := range intervening {
				if ch.Kind == KindDocumentUpdated {
					return &command.ConflictInfo{Collection: CollDocument, ID: ch.ID, Kind: ch.Kind}
				}
			}
		}

		for _, id := range referencedIDs(op.Args) {
			if ch, ok := changed["layer "+id]; ok {
				return &command.ConflictInfo{Collection: CollLayers, ID: id, Kind: ch.Kind}
			}
		}
	}

	return nil
}

// referencedIDs lists the layer identifiers found anywhere in an arguments value, in document order, without
// repeats.
func referencedIDs(args json.RawMessage) []string {
	if len(bytes.TrimSpace(args)) == 0 {
		return nil
	}

	var tree any

	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()

	if err := dec.Decode(&tree); err != nil {
		return nil
	}

	var ids []string

	var walk func(v any)

	walk = func(v any) {
		switch x := v.(type) {
		case string:
			if strings.HasPrefix(x, PrefixLayer+"_") && timeline.ValidID(PrefixLayer, x) && !slices.Contains(ids, x) {
				ids = append(ids, x)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				walk(x[k])
			}
		}
	}

	walk(tree)

	return ids
}
