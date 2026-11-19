package timeline

import (
	"maps"
	"slices"
)

// Graph is what the nesting rules read of the production (SDD-001 §3.5.1): the other timelines, as the
// caller answers them from its own store. The documents it returns are read-only.
type Graph interface {
	// Doc is a timeline of the production by its id.
	Doc(id string) (*Document, bool)
	// Users are the ids of the timelines that hold a composition of id, sorted.
	Users(id string) []string
}

// CompositionTimelines lists, sorted and once each, the timelines the composition items of d name.
func CompositionTimelines(d *Document) []string {
	seen := map[string]bool{}

	for id := range d.Items {
		if it := d.Items[id]; it.Type == ItemComposition && it.Timeline != "" {
			seen[it.Timeline] = true
		}
	}

	return slices.Sorted(maps.Keys(seen))
}

// NestingKind says which rule a placement breaks.
type NestingKind string

// The two rules of nesting.
const (
	NestingCycle NestingKind = "composition_cycle"
	NestingDepth NestingKind = "composition_depth"
)

// Nesting is a placement that breaks a rule. Path is, for a cycle, the ids from the nested timeline to the
// host; Depth is, for the depth rule, the depth the nesting would have.
type Nesting struct {
	Kind  NestingKind
	Path  []string
	Depth int
}

// depthGuard bounds the walks, so a graph that already has a loop (a store edited by hand) ends them.
const depthGuard = 16

// DepthOf is the nesting depth of the timeline id: 0 when it holds no composition, else 1 plus the deepest
// timeline it names. A timeline the graph does not know counts as 0. The walk stops at depthGuard.
func DepthOf(g Graph, id string) int { return depthOf(g, id, 0) }

func depthOf(g Graph, id string, level int) int {
	if level >= depthGuard {
		return level
	}

	d, ok := g.Doc(id)
	if !ok {
		return 0
	}

	return DocumentDepth(g, d, level+1)
}

// DocumentDepth is the depth of a document that may not be in the graph (yet): 0 with no composition, else 1
// plus the deepest timeline its compositions name.
func DocumentDepth(g Graph, d *Document, level int) int {
	deepest, found := 0, false

	for _, child := range CompositionTimelines(d) {
		found = true
		deepest = max(deepest, depthOf(g, child, level))
	}

	if !found {
		return 0
	}

	return deepest + 1
}

// DepthAbove is how many timelines hold id, at the longest chain: 0 when none uses it.
func DepthAbove(g Graph, id string) int { return above(g, id, 0) }

func above(g Graph, id string, level int) int {
	if level >= depthGuard {
		return level
	}

	longest := 0

	for _, u := range g.Users(id) {
		longest = max(longest, 1+above(g, u, level+1))
	}

	return longest
}

// Reaches returns the ids from `from` to `to` through the compositions, `from` first, or nil when `to` cannot
// be reached; a timeline reaches itself with the path [from].
func Reaches(g Graph, from, to string) []string {
	return reach(g, from, to, map[string]bool{})
}

func reach(g Graph, from, to string, seen map[string]bool) []string {
	if from == to {
		return []string{from}
	}

	if seen[from] {
		return nil
	}

	seen[from] = true

	d, ok := g.Doc(from)
	if !ok {
		return nil
	}

	for _, next := range CompositionTimelines(d) {
		if p := reach(g, next, to, seen); p != nil {
			return append([]string{from}, p...)
		}
	}

	return nil
}

// CheckPlacement says whether the timeline host may hold a composition of the timeline child: it must not
// close a loop, and the nesting it makes, counting the timelines above host, the host, and what child holds,
// must stay within MaxCompositionDepth. A nil answer means it may.
func CheckPlacement(g Graph, host, child string) *Nesting {
	if p := Reaches(g, child, host); p != nil {
		return &Nesting{Kind: NestingCycle, Path: p}
	}

	return checkDepth(g, host, DepthOf(g, child))
}

// CheckNewTimeline is CheckPlacement for a timeline that does not exist yet: the new document holds
// compositions of its own (those of the selection that composition.create_from_selection moves), and host will
// hold a composition of it.
func CheckNewTimeline(g Graph, host string, inner *Document) *Nesting {
	return checkDepth(g, host, DocumentDepth(g, inner, 0))
}

func checkDepth(g Graph, host string, below int) *Nesting {
	if total := DepthAbove(g, host) + 1 + below; total > MaxCompositionDepth {
		return &Nesting{Kind: NestingDepth, Depth: total}
	}

	return nil
}
