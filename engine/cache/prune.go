package cache

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// KeepRevisions is how many revisions before the current one keep their frames (SDD-004 section 9).
const KeepRevisions = 5

// Lease marks key as in use: eviction and pruning skip it until the returned function is called.
// A running export holds a lease on every file it reads (SDD-007 9.1), so the cap never takes a
// file from under it. The cache may pass its cap while files are leased.
func (s *Store) Lease(key string) (release func()) {
	s.mu.Lock()
	if s.leases == nil {
		s.leases = map[string]int{}
	}

	s.leases[key]++
	s.mu.Unlock()

	var done bool

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		if done {
			return
		}

		done = true

		s.leases[key]--

		if s.leases[key] <= 0 {
			delete(s.leases, key)
		}
	}
}

// frameRevision parses frames/<project>/<revision>/<file> and reports the project and revision.
func frameRevision(key string) (project string, revision int64, ok bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 4 || parts[0] != "frames" {
		return "", 0, false
	}

	n, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || n < 0 {
		return "", 0, false
	}

	return parts[1], n, true
}

// PruneRevisions deletes the frame directories of project that are older than the newest
// keep+1 revisions, that is the current one and the previous keep. A directory that holds a leased
// file stays. It is called after every frame write with KeepRevisions.
func (s *Store) PruneRevisions(project string, keep int) {
	base, err := s.file(path.Join("frames", project))
	if err != nil {
		return
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}

	var revs []int64

	for _, e := range entries {
		if n, err := strconv.ParseInt(e.Name(), 10, 64); err == nil && e.IsDir() && n >= 0 {
			revs = append(revs, n)
		}
	}

	if len(revs) <= keep+1 {
		return
	}

	sort.Slice(revs, func(i, j int) bool { return revs[i] > revs[j] })

	for _, rev := range revs[keep+1:] {
		s.dropRevision(project, rev)
	}
}

func (s *Store) dropRevision(project string, rev int64) {
	prefix := path.Join("frames", project, strconv.FormatInt(rev, 10)) + "/"

	s.mu.Lock()
	defer s.mu.Unlock()

	for key := range s.leases {
		if strings.HasPrefix(key, prefix) {
			return
		}
	}

	for key, el := range s.items {
		if strings.HasPrefix(key, prefix) {
			s.used -= sizeOf(el)
			s.order.Remove(el)
			delete(s.items, key)
		}
	}

	_ = os.RemoveAll(filepath.Join(s.root, "frames", project, strconv.FormatInt(rev, 10)))
}
