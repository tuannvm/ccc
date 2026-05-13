package watch

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func RunFromArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ccc watch jira [--once] [--dry-run]")
	}
	switch args[0] {
	case "jira":
		return RunJiraFromArgs(context.Background(), args[1:], os.Stdout)
	default:
		return fmt.Errorf("unknown watcher type %q; usage: ccc watch jira [--once] [--dry-run]", args[0])
	}
}

func RunPollFromArgs(args []string) error {
	watchArgs := append([]string{"--once"}, args...)
	return RunJiraFromArgs(context.Background(), watchArgs, os.Stdout)
}

func RunJiraFromArgs(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("ccc watch jira", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	once := fs.Bool("once", false, "run one polling pass")
	dryRun := fs.Bool("dry-run", false, "list eligible issues without claiming or starting sessions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := LoadJiraConfig("")
	if err != nil {
		return err
	}
	provider, err := NewJiraProvider(*cfg)
	if err != nil {
		return fmt.Errorf("create jira client: %w", err)
	}
	interval, err := cfg.PollIntervalDuration()
	if err != nil {
		return err
	}
	if *dryRun {
		runner := &Runner{Provider: provider}
		result, err := runner.RunCycle(ctx, RunOptions{
			Once:               true,
			DryRun:             true,
			MaxTicketsPerCycle: cfg.MaxTicketsPerCycle,
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, FormatCycleResult(result, true))
		return nil
	}
	cccCfg, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("failed to load ccc config: %w", err)
	}
	runner := &Runner{
		Provider:       provider,
		StateStore:     NewFileStateStore(""),
		RepoResolver:   CCCRepoResolver{Config: cccCfg},
		SessionStarter: CCCSessionStarter{},
	}
	if *once {
		result, err := runner.RunCycle(ctx, RunOptions{
			Once:               true,
			MaxTicketsPerCycle: cfg.MaxTicketsPerCycle,
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, FormatCycleResult(result, false))
		return nil
	}
	return runner.Run(ctx, RunOptions{
		Once:               *once,
		DryRun:             *dryRun,
		PollInterval:       int64(interval),
		MaxTicketsPerCycle: cfg.MaxTicketsPerCycle,
	})
}

func FormatCycleResult(result CycleResult, dryRun bool) string {
	if dryRun {
		if len(result.Candidates) == 0 {
			return "No eligible tickets found.\n"
		}
		out := "Eligible tickets:\n"
		for _, t := range result.Candidates {
			out += fmt.Sprintf("- %s %s (%s) repo=%s\n", t.Ref(), t.Title, t.URL, t.RepoRef)
		}
		return out
	}
	return fmt.Sprintf("Started %d ticket session(s), skipped %d ticket(s).\n", len(result.Started), len(result.Skipped))
}
