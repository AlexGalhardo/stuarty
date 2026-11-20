package image

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// sequentialIDs gives the identifiers a command draws, in a fixed order, so a failure replays.
func sequentialIDs() func(prefix string) string {
	n := 0

	return func(prefix string) string {
		n++

		return fmt.Sprintf("%s_z%09d", prefix, n)
	}
}

// FuzzApplyArgs feeds raw bytes as the arguments of any command of the image registry, on the base document
// (SDD-008 section 13.2): the answer is a well formed error that is not E_INTERNAL, or a valid document that its
// inverse takes back to the one before; the document it was given is never changed.
func FuzzApplyArgs(f *testing.F) {
	cmds := Registry().List()
	hostile := `{"layer_id":"lyr_aaaaaaaaaa","x":-1e308,"y":1e308,"w":0,"h":-1,"text":"\u0000","id":"lyr_aaaaaaaaaa"}`

	for i, c := range cmds {
		f.Add(uint8(i), c.Example)
		f.Add(uint8(i), `{}`)
		f.Add(uint8(i), hostile)
	}

	f.Fuzz(func(t *testing.T, which uint8, args string) {
		c := cmds[int(which)%len(cmds)]
		if c.Internal {
			return
		}

		doc := baseDoc()
		ReorderAll(doc)
		frozen := Canonical(doc)

		env := testEnv()
		env.NewIDs = sequentialIDs()

		res, err := ApplyBatch(doc, []command.Operation{{Op: c.Name, Args: []byte(args)}}, env)

		if !bytes.Equal(frozen, Canonical(doc)) {
			t.Fatalf("%s %s: the batch changed the document it was given", c.Name, args)
		}

		if err != nil {
			e := command.AsError(err)
			if e.Code == command.CodeInternal {
				t.Fatalf("%s %s: E_INTERNAL: %v", c.Name, args, e)
			}

			if cerr := command.CheckError(e); cerr != nil {
				t.Fatalf("%s %s: the error breaks the rules of SDD-002 section A.8: %v", c.Name, args, cerr)
			}

			return
		}

		if vs := Validate(res.Doc); len(vs) > 0 {
			t.Fatalf("%s %s: the result is not valid: %v", c.Name, args, vs)
		}

		back, skipped := ApplyPrimitives(res.Doc, res.Inverse, nil)
		if len(skipped) > 0 {
			t.Fatalf("%s %s: the undo skipped %d primitives", c.Name, args, len(skipped))
		}

		if !bytes.Equal(frozen, Canonical(back.Doc)) {
			t.Fatalf("%s %s: undo does not return the previous document", c.Name, args)
		}
	})
}

// FuzzDecode feeds raw bytes to the decoder of an image document: an error, or a document that validates
// and encodes to the same bytes again.
func FuzzDecode(f *testing.F) {
	f.Add(Canonical(baseDoc()))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"schema_version":1,"layers":[{"id":"lyr_aaaaaaaaaa","kind":"fill"}]}`))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		d, err := Decode(raw)
		if err != nil {
			return
		}

		if vs := Validate(d); len(vs) > 0 {
			return // the decoder reads what a file holds; the validator is what judges it
		}

		once := Canonical(d)

		again, err := Decode(once)
		if err != nil {
			t.Fatalf("a canonical document does not decode: %v\n%s", err, once)
		}

		if !bytes.Equal(once, Canonical(again)) {
			t.Fatalf("canonical form is not stable:\n%s\n%s", once, Canonical(again))
		}
	})
}
