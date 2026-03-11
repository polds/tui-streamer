package session

import (
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// Manager owns the full set of active sessions and is safe for concurrent use.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewManager returns an initialised Manager.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
}

// Create allocates a new session with the given display name.
func (m *Manager) Create(name string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := newSession(uuid.New().String(), name)
	m.sessions[s.ID] = s
	return s
}

// Get returns the session with the given ID, or false if it does not exist.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns Info snapshots for all active sessions, sorted by creation time
// (oldest first) so the order is stable across concurrent map iterations.
func (m *Manager) List() []Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]Info, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s.Info())
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list
}

// Delete kills and removes a session. Returns an error if it does not exist.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	s.Kill()
	delete(m.sessions, id)
	return nil
}
