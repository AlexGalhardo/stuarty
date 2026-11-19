// Package cache is the byte-capped, least-recently-used file cache of the frames and bitmaps the
// service renders (SDD-007 section 9.1, threat T-19). Only the files under its root are ever
// removed, and nothing else is: stores, artifacts and exports live elsewhere.
package cache

import (
	"container/list"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultMaxBytes is the default of STUARTY_CACHE_MAX_BYTES: 2 GiB, shared by frames/ and bitmaps/.
const DefaultMaxBytes int64 = 2 << 30

// cacheDirs are the folders of the root that the cache owns.
var cacheDirs = []string{"frames", "bitmaps"}

// ErrKey is returned for a key that is not a plain relative slash path inside the cache.
var ErrKey = errors.New("cache: invalid key")

// FrameKeyHeight is the file of a frame whose height is not the one the width gives: <frame>-<width>x<height>.jpg.
func FrameKeyHeight(project string, revision int64, frame, width, height int) string {
	return path.Join(
		"frames", project, strconv.FormatInt(revision, 10), fmt.Sprintf("%d-%dx%d.jpg", frame, width, height),
	)
}

// FrameKey is the file of a frame: frames/<project>/<revision>/<frame>-<width>.jpg (SDD-000 section 9).
func FrameKey(project string, revision int64, frame, width int) string {
	return path.Join("frames", project, strconv.FormatInt(revision, 10), fmt.Sprintf("%d-%d.jpg", frame, width))
}

// SheetKey is the file of a contact sheet: sheet-<first 12 hex of the key hash>-<width>.jpg in the
// folder of the revision (SDD-003 section 3.11, item 5).
func SheetKey(project string, revision int64, keyHash string, width int) string {
	return path.Join("frames", project, strconv.FormatInt(revision, 10),
		fmt.Sprintf("sheet-%s-%d.jpg", keyHash[:min(12, len(keyHash))], width))
}

// BitmapKey is the file of a rasterized bitmap: bitmaps/<key>.png.
func BitmapKey(key string) string { return path.Join("bitmaps", key+".png") }

type entry struct {
	key  string
	size int64
}

// Store is a directory of cached files with a byte cap. Reading a file makes it the most recently
// used; writing past the cap removes the least recently used files first. It is safe for concurrent use.
type Store struct {
	root string
	max  int64

	mu     sync.Mutex
	leases map[string]int
	used   int64
	order  *list.List // front is the most recently used
	items  map[string]*list.Element
}

// Open uses root as the cache directory, creating it, and indexes what is already there by
// modification time (a read touches the file, so that is the order of use). A cap of zero or less is
// DefaultMaxBytes. Files over the cap are evicted at once.
func Open(root string, maxBytes int64) (*Store, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	s := &Store{root: root, max: maxBytes, order: list.New(), items: map[string]*list.Element{}}

	type found struct {
		entry
		mod time.Time
	}
	var all []found
	// Only the cache folders are indexed: the root may be the workspace, which holds the database too.
	for _, sub := range cacheDirs {
		err := filepath.WalkDir(filepath.Join(root, sub), func(p string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil || d.IsDir() {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			all = append(all, found{entry{filepath.ToSlash(rel), info.Size()}, info.ModTime()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mod.Before(all[j].mod) })
	for _, f := range all {
		s.items[f.key] = s.order.PushFront(&entry{f.key, f.size})
		s.used += f.size
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evict()
	return s, nil
}

// Size is the bytes the cache holds.
func (s *Store) Size() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}

// Max is the cap in bytes.
func (s *Store) Max() int64 { return s.max }

// file is the path of a key, refusing anything that leaves the root.
func (s *Store) file(key string) (string, error) {
	if key == "" || strings.Contains(key, `\`) || path.IsAbs(key) || path.Clean(key) != key ||
		key == "." || key == ".." || strings.HasPrefix(key, "../") || strings.Contains(key, ":") {
		return "", fmt.Errorf("%w: %q", ErrKey, key)
	}
	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}

// Get returns the cached bytes of key and marks them most recently used.
func (s *Store) Get(key string) ([]byte, bool) {
	p, err := s.file(key)
	if err != nil {
		return nil, false
	}
	s.mu.Lock()
	el, ok := s.items[key]
	if ok {
		s.order.MoveToFront(el)
	}
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		s.forget(key)
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now) // so the order survives a restart
	return b, true
}

// Put stores data under key, replacing what was there, and evicts least recently used files until the
// cache fits its cap again. Data larger than the cap is not stored.
func (s *Store) Put(key string, data []byte) error {
	p, err := s.file(key)
	if err != nil {
		return err
	}
	if int64(len(data)) > s.max {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".part"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	s.mu.Lock()
	if el, ok := s.items[key]; ok {
		s.used -= sizeOf(el)
		s.order.Remove(el)
	}
	s.items[key] = s.order.PushFront(&entry{key, int64(len(data))})
	s.used += int64(len(data))
	s.evict()
	s.mu.Unlock()

	if project, _, ok := frameRevision(key); ok {
		s.PruneRevisions(project, KeepRevisions)
	}

	return nil
}

func sizeOf(el *list.Element) int64 {
	if e, ok := el.Value.(*entry); ok {
		return e.size
	}
	return 0
}

// forget drops a key whose file is gone.
func (s *Store) forget(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.items[key]; ok {
		s.used -= sizeOf(el)
		s.order.Remove(el)
		delete(s.items, key)
	}
}

// evict removes least recently used files until the cache fits. The caller holds the lock.
func (s *Store) evict() {
	for el := s.order.Back(); el != nil && s.used > s.max; {
		prev := el.Prev()
		e, ok := el.Value.(*entry)

		// A leased file is being read by a running export: it stays, and the next one goes.
		if ok && s.leases[e.key] > 0 {
			el = prev

			continue
		}

		s.order.Remove(el)
		el = prev

		if !ok {
			continue
		}
		delete(s.items, e.key)
		s.used -= e.size
		if p, err := s.file(e.key); err == nil {
			_ = os.Remove(p)
		}
	}
}
