package web

import (
	"sync"
	"time"
)

// SessionState represents the current state of a session.
type SessionState string

// session state constants.
const (
	SessionStateActive    SessionState = "active"    // session is running (progress file locked)
	SessionStateCompleted SessionState = "completed" // session finished (no lock held)
)

// SessionMetadata holds parsed information from progress file header.
type SessionMetadata struct {
	PlanPath  string    // path to plan file (from "Plan:" header line)
	Branch    string    // git branch (from "Branch:" header line)
	Mode      string    // execution mode: full, review, codex-only (from "Mode:" header line)
	StartTime time.Time // start time (from "Started:" header line)
}

// Session represents a single ralphex execution instance.
// each session corresponds to one progress file and maintains its own event buffer and hub.
type Session struct {
	mu sync.RWMutex

	ID       string          // unique identifier (derived from progress filename)
	Path     string          // full path to progress file
	Metadata SessionMetadata // parsed header information
	State    SessionState    // current state (active/completed)
	Buffer   *Buffer         // event buffer for this session
	Hub      *Hub            // event hub for SSE streaming
	Tailer   *Tailer         // file tailer for reading new content (nil if not tailing)

	// lastModified tracks the file's last modification time for change detection
	lastModified time.Time

	// stopTailCh signals the tail feeder goroutine to stop
	stopTailCh chan struct{}
}

// NewSession creates a new session for the given progress file path.
// the session starts with an empty buffer and hub; metadata should be populated
// by calling ParseMetadata after creation.
func NewSession(id, path string) *Session {
	return &Session{
		ID:     id,
		Path:   path,
		State:  SessionStateCompleted, // default to completed until proven active
		Buffer: NewBuffer(DefaultBufferSize),
		Hub:    NewHub(),
	}
}

// SetMetadata updates the session's metadata thread-safely.
func (s *Session) SetMetadata(meta SessionMetadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Metadata = meta
}

// GetMetadata returns the session's metadata thread-safely.
func (s *Session) GetMetadata() SessionMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Metadata
}

// SetState updates the session's state thread-safely.
func (s *Session) SetState(state SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// GetState returns the session's state thread-safely.
func (s *Session) GetState() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// SetLastModified updates the last modified time thread-safely.
func (s *Session) SetLastModified(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastModified = t
}

// GetLastModified returns the last modified time thread-safely.
func (s *Session) GetLastModified() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastModified
}

// StartTailing begins tailing the progress file and feeding events to buffer/hub.
// if fromStart is true, reads from the beginning of the file; otherwise from the end.
// does nothing if already tailing.
func (s *Session) StartTailing(fromStart bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Tailer != nil && s.Tailer.IsRunning() {
		return nil // already tailing
	}

	s.Tailer = NewTailer(s.Path, DefaultTailerConfig())
	if err := s.Tailer.Start(fromStart); err != nil {
		s.Tailer = nil
		return err
	}

	s.stopTailCh = make(chan struct{})
	go s.feedEvents()

	return nil
}

// StopTailing stops the tailer and event feeder goroutine.
func (s *Session) StopTailing() {
	s.mu.Lock()
	if s.stopTailCh != nil {
		close(s.stopTailCh)
		s.stopTailCh = nil
	}
	tailer := s.Tailer
	s.mu.Unlock()

	if tailer != nil {
		tailer.Stop()
	}
}

// IsTailing returns whether the session is currently tailing its progress file.
func (s *Session) IsTailing() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Tailer != nil && s.Tailer.IsRunning()
}

// feedEvents reads events from the tailer and adds them to buffer/hub.
func (s *Session) feedEvents() {
	s.mu.RLock()
	tailer := s.Tailer
	stopCh := s.stopTailCh
	s.mu.RUnlock()

	if tailer == nil {
		return
	}

	eventCh := tailer.Events()
	for {
		select {
		case <-stopCh:
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			s.Buffer.Add(event)
			s.Hub.Broadcast(event)
		}
	}
}

// Close cleans up session resources including the tailer.
func (s *Session) Close() {
	s.StopTailing()
	s.Hub.Close()
	s.Buffer.Clear()
}
