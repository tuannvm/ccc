package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

type State struct {
	Tickets map[string]*StateEntry `json:"tickets"`
}

type StateEntry struct {
	Provider    string    `json:"provider"`
	TicketKey   string    `json:"ticket_key"`
	RepoPath    string    `json:"repo_path,omitempty"`
	SessionName string    `json:"session_name,omitempty"`
	TopicID     int64     `json:"topic_id,omitempty"`
	ClaimedAt   time.Time `json:"claimed_at,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

func (s *State) ensure() {
	if s.Tickets == nil {
		s.Tickets = make(map[string]*StateEntry)
	}
}

func StateKey(provider, ticketRef string) string {
	return provider + ":" + ticketRef
}

type FileStateStore struct {
	Path string
}

func DefaultStatePath() string {
	return filepath.Join(configpkg.ConfigDir(), "watch-state.json")
}

func NewFileStateStore(path string) *FileStateStore {
	if path == "" {
		path = DefaultStatePath()
	}
	return &FileStateStore{Path: path}
}

func (s *FileStateStore) Load(ctx context.Context) (*State, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Tickets: make(map[string]*StateEntry)}, nil
		}
		return nil, fmt.Errorf("load watcher state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse watcher state %s: %w", s.Path, err)
	}
	state.ensure()
	return &state, nil
}

func (s *FileStateStore) Save(ctx context.Context, state *State) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if state == nil {
		state = &State{}
	}
	state.ensure()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return fmt.Errorf("create watcher state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode watcher state: %w", err)
	}
	data = append(data, '\n')
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write watcher state: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("save watcher state: %w", err)
	}
	return nil
}
