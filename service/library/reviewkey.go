package library

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// reviewKeyFile holds the key that seals the review marks of the skills of this
// library (SDD-018 section 7): 32 random bytes in hex. It is a dot file of
// library/, so no scan, import or export reads it, and a skill folder copied in
// by hand cannot carry it.
const reviewKeyFile = ".review-key"

// reviewKey returns the key of the library. With create false a missing or
// unreadable key is nil, so no skill reads as reviewed; with create true a key
// is made when there is none (the first review of the workspace). A key that is
// not valid is replaced, which takes every earlier review away: the safe side.
func (m *Manager) reviewKey(create bool) ([]byte, error) {
	m.keyMu.Lock()
	defer m.keyMu.Unlock()

	if m.key != nil {
		return m.key, nil
	}

	root, err := m.files.open()
	if err != nil {
		return nil, err
	}

	if root != nil {
		data, _, rerr := readSmall(root, reviewKeyFile, 256)
		_ = root.Close()

		if rerr == nil {
			if key, derr := hex.DecodeString(strings.TrimSpace(string(data))); derr == nil && len(key) == 32 {
				m.key = key

				return key, nil
			}
		}
	}

	if !create {
		return nil, nil //nolint:nilnil // no key yet: nothing is reviewed
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("library: the review key cannot be made: %w", err)
	}

	root, err = m.files.create()
	if err != nil {
		return nil, err
	}

	defer func() { _ = root.Close() }()

	if err := writeFile(root, reviewKeyFile, []byte(hex.EncodeToString(key)+"\n")); err != nil {
		return nil, err
	}

	m.key = key

	return key, nil
}

// sealReview marks the skill as read by a person, with the key of the library.
func (m *Manager) sealReview(s *core.Skill) error {
	key, err := m.reviewKey(true)
	if err != nil {
		return err
	}

	s.MarkReviewed(key)

	return nil
}

// Review marks a skill value as read with the key of this library, for a caller
// that writes it next with WriteSkill (the tests and the install of the skills
// that ship with the product). It is not an operation of the service: a review
// by a person goes through MarkReviewed.
func (m *Manager) Review(s *core.Skill) error { return m.sealReview(s) }

// isReviewed reports whether the skill carries a review sealed by this library.
func (m *Manager) isReviewed(s *core.Skill) bool {
	key, err := m.reviewKey(false)
	if err != nil || key == nil {
		return false
	}

	return s.Reviewed(key)
}
