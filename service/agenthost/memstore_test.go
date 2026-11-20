package agenthost

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

// memStore is a Store in memory, for the tests of the host.
type memStore struct {
	mu       sync.Mutex
	profiles map[string]Profile
	sessions map[string]Session
	events   map[string]map[int64]chat.Event
}

func newMemStore() *memStore {
	return &memStore{
		profiles: map[string]Profile{}, sessions: map[string]Session{}, events: map[string]map[int64]chat.Event{},
	}
}

func (m *memStore) ProfileList(context.Context) ([]Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Profile
	for _, p := range m.profiles {
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

func (m *memStore) ProfileGet(_ context.Context, name string) (*Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.profiles[name]
	if !ok {
		return nil, ErrNotFound
	}

	return &p, nil
}

func (m *memStore) ProfileSet(_ context.Context, p Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.profiles[p.Name] = p

	return nil
}

func (m *memStore) ProfileRemove(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.profiles[name]; !ok {
		return ErrNotFound
	}

	delete(m.profiles, name)

	return nil
}

func (m *memStore) SessionCreate(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[s.ID] = s

	return nil
}

func (m *memStore) SessionUpdate(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[s.ID]; !ok {
		return ErrNotFound
	}

	m.sessions[s.ID] = s

	return nil
}

func (m *memStore) SessionGet(_ context.Context, id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}

	return &s, nil
}

func (m *memStore) SessionList(_ context.Context, projectID string) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Session

	for id := range m.sessions {
		if m.sessions[id].ProjectID == projectID {
			out = append(out, m.sessions[id])
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })

	return out, nil
}

func (m *memStore) SessionDelete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, id)
	delete(m.events, id)

	return nil
}

func (m *memStore) SessionsEndLive(_ context.Context, at time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := 0

	for id := range m.sessions {
		s := m.sessions[id]
		if s.State != chat.StateEnded {
			s.State, s.EndReason, s.EndedAt = chat.StateEnded, chat.EndShutdown, &at
			m.sessions[id] = s
			n++
		}
	}

	return n, nil
}

func (m *memStore) SessionsPrune(context.Context, string, int) error { return nil }

func (m *memStore) EventUpsert(_ context.Context, sessionID string, e chat.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.events[sessionID] == nil {
		m.events[sessionID] = map[int64]chat.Event{}
	}

	m.events[sessionID][e.Seq] = e

	return nil
}

func (m *memStore) EventList(_ context.Context, sessionID string, after int64, limit int) ([]chat.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []chat.Event

	for seq, e := range m.events[sessionID] {
		if seq > after {
			out = append(out, e)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })

	if len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}
