package service_test

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/untrusted"
)

// walk collects the "package.Type.Field" of every field tagged
// untrusted:"true" that a result type reaches.
func walk(t reflect.Type, seen map[reflect.Type]bool, out map[string]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map || t.Kind() == reflect.Array {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct || seen[t] || t.PkgPath() == "" {
		return
	}

	seen[t] = true

	for f := range t.Fields() {
		if f.Tag.Get("untrusted") == "true" {
			out[t.PkgPath()[strings.LastIndex(t.PkgPath(), "/")+1:]+"."+t.Name()+"."+f.Name] = true
		}

		if f.Anonymous || f.IsExported() {
			walk(f.Type, seen, out)
		}
	}
}

// RF-133, SDD-007 section 8.1: every field of a Service result that carries
// text from media or from another actor is in the provenance table, so it is
// delimited with a known source; and the table lists nothing that no longer
// exists.
func TestProvenanceTableListsEveryUntrustedField(t *testing.T) {
	found := map[string]bool{}
	seen := map[reflect.Type]bool{}

	for m := range reflect.TypeFor[service.Service]().Methods() {
		for out := range m.Type.Outs() {
			walk(out, seen, found)
		}
	}

	var missing, stale []string

	for name := range found {
		if _, ok := untrusted.Provenance[name]; !ok {
			missing = append(missing, name)
		}
	}

	for name, source := range untrusted.Provenance {
		if !found[name] {
			stale = append(stale, name)
		}

		if !slices.Contains(untrusted.Sources, source) {
			t.Errorf("%s names the unknown source %q", name, source)
		}
	}

	sort.Strings(missing)
	sort.Strings(stale)

	if len(missing) > 0 {
		t.Errorf("fields tagged untrusted that the provenance table does not list: %v", missing)
	}

	if len(stale) > 0 {
		t.Errorf("provenance entries with no such field: %v", stale)
	}
}
