package watch

import "context"

// Ticket is the ticket-system-neutral shape the watcher core operates on.
type Ticket struct {
	ID                 string
	Key                string
	Title              string
	Description        string
	URL                string
	RepoRef            string
	AcceptanceCriteria string
	Provider           string
}

func (t Ticket) Ref() string {
	if t.Key != "" {
		return t.Key
	}
	return t.ID
}

// TicketProvider supplies candidate tickets and provider-specific operations.
type TicketProvider interface {
	Name() string
	PollCandidates(ctx context.Context, max int) ([]Ticket, error)
	Claim(ctx context.Context, ticket Ticket) error
	FetchContext(ctx context.Context, ticket Ticket) (Ticket, error)
	Comment(ctx context.Context, ticket Ticket, message string) error
}

type StateStore interface {
	Load(ctx context.Context) (*State, error)
	Save(ctx context.Context, state *State) error
}

type RepoResolver interface {
	Resolve(ctx context.Context, repoRef string) (ResolvedRepo, error)
}

type SessionStarter interface {
	Start(ctx context.Context, sessionName, workDir, prompt string) (SessionStartResult, error)
}

type ResolvedRepo struct {
	Path string
	Name string
}

type SessionStartResult struct {
	SessionName string
	TopicID     int64
}

type RunOptions struct {
	Once               bool
	DryRun             bool
	PollInterval       int64
	MaxTicketsPerCycle int
}

type CycleResult struct {
	Candidates []Ticket
	Started    []StateEntry
	Skipped    []SkippedTicket
}

type SkippedTicket struct {
	Ticket Ticket
	Reason string
}
