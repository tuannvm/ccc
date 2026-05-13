package watch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	candidates []Ticket
	claimed    []string
	fetchErr   error
}

func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) PollCandidates(context.Context, int) ([]Ticket, error) {
	return p.candidates, nil
}
func (p *fakeProvider) Claim(_ context.Context, t Ticket) error {
	p.claimed = append(p.claimed, t.Ref())
	return nil
}
func (p *fakeProvider) FetchContext(_ context.Context, t Ticket) (Ticket, error) {
	if p.fetchErr != nil {
		return Ticket{}, p.fetchErr
	}
	if t.RepoRef == "" {
		t.RepoRef = "/repo"
	}
	return t, nil
}
func (p *fakeProvider) Comment(context.Context, Ticket, string) error { return nil }

type memoryStateStore struct {
	state *State
}

func (s *memoryStateStore) Load(context.Context) (*State, error) {
	if s.state == nil {
		s.state = &State{Tickets: map[string]*StateEntry{}}
	}
	return s.state, nil
}
func (s *memoryStateStore) Save(_ context.Context, state *State) error {
	s.state = state
	return nil
}

type fakeResolver struct {
	path string
	name string
}

func (r fakeResolver) Resolve(context.Context, string) (ResolvedRepo, error) {
	return ResolvedRepo{Path: r.path, Name: r.name}, nil
}

type fakeStarter struct {
	err     error
	started []string
}

func (s *fakeStarter) Start(_ context.Context, sessionName, workDir, prompt string) (SessionStartResult, error) {
	s.started = append(s.started, sessionName+"|"+workDir+"|"+prompt)
	if s.err != nil {
		return SessionStartResult{}, s.err
	}
	return SessionStartResult{SessionName: sessionName, TopicID: 42}, nil
}

func TestRunnerNoCandidates(t *testing.T) {
	runner := &Runner{
		Provider:       &fakeProvider{},
		StateStore:     &memoryStateStore{},
		RepoResolver:   fakeResolver{path: "/repo", name: "repo"},
		SessionStarter: &fakeStarter{},
	}
	result, err := runner.RunCycle(context.Background(), RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 0 || len(result.Started) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunnerSkipsExistingState(t *testing.T) {
	store := &memoryStateStore{state: &State{Tickets: map[string]*StateEntry{
		StateKey("fake", "ABC-1"): {TicketKey: "ABC-1"},
	}}}
	provider := &fakeProvider{candidates: []Ticket{{Key: "ABC-1", Title: "skip", RepoRef: "/repo"}}}
	runner := &Runner{Provider: provider, StateStore: store, RepoResolver: fakeResolver{}, SessionStarter: &fakeStarter{}}
	result, err := runner.RunCycle(context.Background(), RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %d, want 1", len(result.Skipped))
	}
	if len(provider.claimed) != 0 {
		t.Fatalf("claimed existing ticket: %+v", provider.claimed)
	}
}

func TestRunnerClaimsAndStartsSession(t *testing.T) {
	provider := &fakeProvider{candidates: []Ticket{{Key: "ABC-2", Title: "Build it", RepoRef: "/repo", Description: "desc"}}}
	starter := &fakeStarter{}
	runner := &Runner{
		Provider:       provider,
		StateStore:     &memoryStateStore{},
		RepoResolver:   fakeResolver{path: "/tmp/repo", name: "repo.name"},
		SessionStarter: starter,
		Now:            func() time.Time { return time.Unix(100, 0).UTC() },
	}
	result, err := runner.RunCycle(context.Background(), RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.claimed) != 1 || provider.claimed[0] != "ABC-2" {
		t.Fatalf("claimed = %+v", provider.claimed)
	}
	if len(result.Started) != 1 {
		t.Fatalf("started = %d, want 1", len(result.Started))
	}
	if !strings.HasPrefix(starter.started[0], "abc-2-repo-name|/tmp/repo|") {
		t.Fatalf("unexpected session start: %s", starter.started[0])
	}
}

func TestRunnerPreservesErrorStateWhenSessionStartupFails(t *testing.T) {
	store := &memoryStateStore{}
	provider := &fakeProvider{candidates: []Ticket{{Key: "ABC-3", Title: "Fail", RepoRef: "/repo"}}}
	runner := &Runner{
		Provider:       provider,
		StateStore:     store,
		RepoResolver:   fakeResolver{path: "/tmp/repo", name: "repo"},
		SessionStarter: &fakeStarter{err: errors.New("tmux failed")},
	}
	_, err := runner.RunCycle(context.Background(), RunOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	entry := store.state.Tickets[StateKey("fake", "ABC-3")]
	if entry == nil {
		t.Fatal("missing state entry")
	}
	if entry.LastError != "tmux failed" || entry.SessionName != "abc-3-repo" {
		t.Fatalf("entry = %+v", entry)
	}
}
