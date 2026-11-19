package command

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// determinismOut names a file that receives one "name sha256" line per case, sorted by name, so
// that the CI runners of the three operating systems can be compared byte for byte (NFR-025).
const determinismOut = "VUKEM_DETERMINISM_OUT"

const (
	determinismSeeds = 64
	determinismSteps = 60
)

// writeResult feeds the hash with everything a command returned: the canonical document, the
// inverse, the diff and the effective arguments on success, the whole error on failure.
func writeResult(t *testing.T, h hash.Hash, res Result, err error) {
	t.Helper()

	if err != nil {
		b, jerr := json.Marshal(AsError(err))
		if jerr != nil {
			t.Fatal(jerr)
		}

		h.Write([]byte("error\n"))
		h.Write(b)

		return
	}

	h.Write([]byte("ok\n"))
	h.Write(timeline.Canonical(res.Doc))

	for _, v := range []any{res.Inverse, res.Diff, res.Effective} {
		b, jerr := json.Marshal(v)
		if jerr != nil {
			t.Fatal(jerr)
		}

		h.Write([]byte{'\n'})
		h.Write(b)
	}
}

func goldenHashes(t *testing.T, root string, into map[string]string) {
	t.Helper()

	for _, cmd := range fullRegistry().List() {
		cases, err := os.ReadDir(filepath.Join(root, cmd.Name))
		if err != nil {
			continue
		}

		for _, c := range cases {
			if !c.IsDir() {
				continue
			}

			dir := filepath.Join(root, cmd.Name, c.Name())
			reg := fullRegistry()

			before, err := os.ReadFile(filepath.Join(dir, "before.json"))
			if err != nil {
				t.Fatal(err)
			}

			doc, err := timeline.Decode(before)
			if err != nil {
				t.Fatalf("%s: %v", dir, err)
			}

			opBytes, err := os.ReadFile(filepath.Join(dir, "op.json"))
			if err != nil {
				t.Fatal(err)
			}

			var op Operation
			if err := json.Unmarshal(opBytes, &op); err != nil {
				t.Fatal(err)
			}

			var generated []string
			if b, err := os.ReadFile(filepath.Join(dir, "newids.json")); err == nil {
				if err := json.Unmarshal(b, &generated); err != nil {
					t.Fatal(err)
				}
			}

			res, applyErr := reg.Apply(doc, op, Env{NewIDs: func(string) string {
				id := generated[0]
				generated = generated[1:]

				return id
			}})

			h := sha256.New()
			writeResult(t, h, res, applyErr)
			into["golden/"+cmd.Name+"/"+c.Name()] = hex.EncodeToString(h.Sum(nil))
		}
	}
}

// randomHashes runs seeded random batches of the whole registry and hashes every step.
func randomHashes(t *testing.T, into map[string]string) {
	t.Helper()

	for seed := int64(1); seed <= determinismSeeds; seed++ {
		rng := rand.New(rand.NewSource(seed))
		doc := randomStart(rng)
		ids := &seqIDs{}
		h := sha256.New()

		for i := range determinismSteps {
			o := randomRegistryOp(rng, doc)
			res, err := fullRegistry().Apply(doc, o, Env{NewIDs: ids.next})

			h.Write([]byte(o.Op))
			h.Write(o.Args)
			writeResult(t, h, res, err)

			if err == nil {
				doc = res.Doc
			}

			if i%7 == 6 || len(doc.Items) > 80 {
				doc = randomStart(rng)
			}
		}

		into[fmt.Sprintf("random/seed-%03d", seed)] = hex.EncodeToString(h.Sum(nil))
	}
}

// TestCanonicalHashesAreStable hashes the golden documents and the seeded random batches twice
// and requires the same answer; with VUKEM_DETERMINISM_OUT set it writes the hashes to that file
// for the cross-platform comparison of the CI (a map iteration, a float format or a path
// separator that leaks into a result shows up as a different hash on one runner).
func TestCanonicalHashesAreStable(t *testing.T) {
	first, second := map[string]string{}, map[string]string{}

	goldenHashes(t, "testdata", first)
	randomHashes(t, first)
	goldenHashes(t, "testdata", second)
	randomHashes(t, second)

	if len(first) < determinismSeeds+100 {
		t.Fatalf("only %d hashes; the golden cases were not found", len(first))
	}

	names := make([]string, 0, len(first))
	for name, sum := range first {
		names = append(names, name)

		if second[name] != sum {
			t.Errorf("%s: two runs gave different hashes", name)
		}
	}

	sort.Strings(names)

	out := os.Getenv(determinismOut)
	if out == "" {
		return
	}

	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "%s %s\n", name, first[name])
	}

	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}
