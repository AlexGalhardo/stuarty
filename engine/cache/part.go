package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PartPath is the path a file of key is written to before Commit: the final path with ".part" after it, in the folder
// of the key, which is made. A writer that is too big for the cache, or that fails, leaves only that name, which the
// next start indexes like any other file (SDD-011 section 6.4).
func (s *Store) PartPath(key string) (string, error) {
	p, err := s.file(key)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}

	return p + ".part", nil
}

// Commit renames the file written at PartPath(key) to its name, and counts its size, so that the cache evicts least
// recently used files until it fits its cap again. A file larger than the cap is removed and is not stored.
func (s *Store) Commit(key string) error {
	p, err := s.file(key)
	if err != nil {
		return err
	}

	info, err := os.Stat(p + ".part")
	if err != nil {
		return err
	}

	if info.Size() > s.max {
		_ = os.Remove(p + ".part")

		return fmt.Errorf("cache: the file of %q is larger than the cache", key)
	}

	if err := os.Rename(p+".part", p); err != nil {
		_ = os.Remove(p + ".part")

		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if el, ok := s.items[key]; ok {
		s.used -= sizeOf(el)
		s.order.Remove(el)
	}

	s.items[key] = s.order.PushFront(&entry{key, info.Size()})
	s.used += info.Size()
	s.evict()

	return nil
}

// Has reports whether key is stored, and makes it the most recently used. It reads no byte.
func (s *Store) Has(key string) bool {
	p, err := s.file(key)
	if err != nil {
		return false
	}

	s.mu.Lock()
	el, ok := s.items[key]
	if ok {
		s.order.MoveToFront(el)
	}
	s.mu.Unlock()

	if !ok {
		return false
	}

	if _, err := os.Stat(p); err != nil {
		s.forget(key)

		return false
	}

	now := time.Now()
	_ = os.Chtimes(p, now, now)

	return true
}
