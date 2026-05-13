package watch

import (
	"context"
	"fmt"
	"time"
)

type Runner struct {
	Provider       TicketProvider
	StateStore     StateStore
	RepoResolver   RepoResolver
	SessionStarter SessionStarter
	Now            func() time.Time
}

func (r *Runner) Run(ctx context.Context, opts RunOptions) error {
	if opts.MaxTicketsPerCycle <= 0 {
		opts.MaxTicketsPerCycle = 1
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = int64(time.Minute)
	}
	for {
		if _, err := r.RunCycle(ctx, opts); err != nil {
			return err
		}
		if opts.Once {
			return nil
		}
		timer := time.NewTimer(time.Duration(opts.PollInterval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *Runner) RunCycle(ctx context.Context, opts RunOptions) (CycleResult, error) {
	if r.Provider == nil {
		return CycleResult{}, fmt.Errorf("ticket provider is required")
	}
	if opts.MaxTicketsPerCycle <= 0 {
		opts.MaxTicketsPerCycle = 1
	}
	candidates, err := r.Provider.PollCandidates(ctx, opts.MaxTicketsPerCycle)
	if err != nil {
		return CycleResult{}, fmt.Errorf("poll %s tickets: %w", r.Provider.Name(), err)
	}
	result := CycleResult{Candidates: candidates}
	if opts.DryRun {
		return result, nil
	}
	if r.StateStore == nil {
		return CycleResult{}, fmt.Errorf("state store is required")
	}
	state, err := r.StateStore.Load(ctx)
	if err != nil {
		return result, err
	}
	state.ensure()
	for _, candidate := range candidates {
		ref := candidate.Ref()
		key := StateKey(r.Provider.Name(), ref)
		if _, exists := state.Tickets[key]; exists {
			result.Skipped = append(result.Skipped, SkippedTicket{Ticket: candidate, Reason: "already in local watcher state"})
			continue
		}
		if r.RepoResolver == nil {
			return result, fmt.Errorf("repo resolver is required")
		}
		if r.SessionStarter == nil {
			return result, fmt.Errorf("session starter is required")
		}
		full, err := r.Provider.FetchContext(ctx, candidate)
		if err != nil {
			return result, fmt.Errorf("fetch context for %s: %w", ref, err)
		}
		if full.Provider == "" {
			full.Provider = r.Provider.Name()
		}
		repo, err := r.RepoResolver.Resolve(ctx, full.RepoRef)
		if err != nil {
			return result, fmt.Errorf("resolve repo for %s: %w", ref, err)
		}
		if err := r.Provider.Claim(ctx, full); err != nil {
			return result, fmt.Errorf("claim %s: %w", ref, err)
		}
		now := r.now()
		entry := &StateEntry{
			Provider:  r.Provider.Name(),
			TicketKey: ref,
			RepoPath:  repo.Path,
			ClaimedAt: now,
		}
		state.Tickets[key] = entry
		if err := r.StateStore.Save(ctx, state); err != nil {
			return result, err
		}
		sessionName := SessionName(ref, repo.Name)
		start, err := r.SessionStarter.Start(ctx, sessionName, repo.Path, BuildInitialPrompt(full))
		if err != nil {
			entry.SessionName = sessionName
			entry.LastError = err.Error()
			_ = r.StateStore.Save(ctx, state)
			return result, fmt.Errorf("start session for %s: %w", ref, err)
		}
		entry.SessionName = sessionName
		entry.TopicID = start.TopicID
		entry.StartedAt = r.now()
		entry.LastError = ""
		if err := r.StateStore.Save(ctx, state); err != nil {
			return result, err
		}
		result.Started = append(result.Started, *entry)
	}
	return result, nil
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}
