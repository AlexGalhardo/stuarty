package timeline

import (
	"bytes"
	"encoding/json"
)

// Full returns doc as a RawDoc in the form a client fetches: the canonical tree of
// Raw plus every field that has a default, written out (scale, opacity, filter,
// settings.width, and so on). An edit diff built from Full trees patches a fetched
// document into the stored one with no derivation (SDD-002 §A.5).
func Full(doc *Document) RawDoc {
	out, _ := fill(map[string]any(Raw(doc)), marshalTree(doc)).(map[string]any)

	return RawDoc(out)
}

// FullValue is Full for one object of a document (an item, a track, an asset, a font).
func FullValue(v any) any { return fill(RawValue(v), marshalTree(v)) }

// marshalTree is the decoded JSON of v as encoding/json writes it: fields without
// omitempty are present even when equal to their default.
func marshalTree(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		panic("timeline: document is not encodable: " + err.Error())
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var out any
	if err := dec.Decode(&out); err != nil {
		panic("timeline: " + err.Error())
	}

	return out
}

// fill returns canon with the keys that only full has added, at every level. The
// canonical value wins where both have a key, so a field that encoding/json drops
// (an unknown field) stays.
func fill(canon, full any) any {
	switch c := canon.(type) {
	case map[string]any:
		f, ok := full.(map[string]any)
		if !ok {
			return canon
		}

		for k, fv := range f {
			if cv, has := c[k]; has {
				c[k] = fill(cv, fv)
			} else if fv != nil {
				c[k] = fv
			}
		}

		return c
	case []any:
		f, ok := full.([]any)
		if ok && len(f) == len(c) {
			for i := range c {
				c[i] = fill(c[i], f[i])
			}
		}

		return c
	}

	return canon
}
