package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/polds/tui-streamer/internal/executor"
)

// maxLineBuf is the maximum number of output lines retained for replay to
// clients that connect after execution has already started.
const maxLineBuf = 10_000

// Session is a named execution context. Multiple WebSocket clients can
// subscribe and all receive the same streamed output from any executed command.
type Session struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	// PendingCommand is an optional pre-configured command string loaded from a
	// bundle. It is surfaced to the UI so the command bar can be pre-populated.
	// An empty string means no pending command.
	PendingCommand string    `json:"pending_command,omitempty"`
	// BundleName is the name of the bundle this session belongs to, if any.
	BundleName     string    `json:"bundle_name,omitempty"`

	mu      sync.RWMutex
	clients map[*Client]struct{}
	running bool
	cancel  context.CancelFunc
	lineBuf [][]byte // buffered output lines replayed to late-joining clients
}

// Info is a JSON-safe snapshot of the session's current state.
type Info struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	Running        bool      `json:"running"`
	ClientCount    int       `json:"client_count"`
	PendingCommand string    `json:"pending_command,omitempty"`
	BundleName     string    `json:"bundle_name,omitempty"`
}

func newSession(id, name, bundleName string) *Session {
	return &Session{
		ID:         id,
		Name:       name,
		BundleName: bundleName,
		CreatedAt:  time.Now(),
		clients:    make(map[*Client]struct{}),
	}
}

// Info returns a point-in-time snapshot suitable for JSON serialisation.
func (s *Session) Info() Info {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Info{
		ID:             s.ID,
		Name:           s.Name,
		CreatedAt:      s.CreatedAt,
		Running:        s.running,
		ClientCount:    len(s.clients),
		PendingCommand: s.PendingCommand,
		BundleName:     s.BundleName,
	}
}

// Exec starts a command in the session and streams its output to all
// subscribed clients. Returns an error if a command is already running.
func (s *Session) Exec(opts executor.Options) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("session already has a running command")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	s.lineBuf = nil // clear replay buffer for the new command
	s.mu.Unlock()

	lines, err := executor.Run(ctx, opts)
	if err != nil {
		s.mu.Lock()
		s.running = false
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		return err
	}

	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.cancel = nil
			s.mu.Unlock()
			cancel()
		}()
		for line := range lines {
			data, err := json.Marshal(line)
			if err != nil {
				continue
			}
			s.broadcast(data)
		}
	}()

	return nil
}

// Kill terminates any running command in the session.
func (s *Session) Kill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Session) subscribe(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c] = struct{}{}
	// Replay buffered lines so late-joining clients see prior output.
	for _, line := range s.lineBuf {
		c.Send(line)
	}
}

func (s *Session) unsubscribe(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, c)
}

func (s *Session) broadcast(msg []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lineBuf) < maxLineBuf {
		s.lineBuf = append(s.lineBuf, msg)
	}
	for c := range s.clients {
		c.Send(msg)
	}
}
