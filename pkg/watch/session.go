package watch

import (
	"context"
	"fmt"

	listenpkg "github.com/tuannvm/ccc/pkg/listen"
)

type CCCSessionStarter struct{}

func (CCCSessionStarter) Start(ctx context.Context, sessionName, workDir, prompt string) (SessionStartResult, error) {
	select {
	case <-ctx.Done():
		return SessionStartResult{}, ctx.Err()
	default:
	}
	info, err := listenpkg.StartDetachedWithResult(sessionName, workDir, prompt)
	if err != nil {
		return SessionStartResult{}, err
	}
	return SessionStartResult{SessionName: sessionName, TopicID: info.TopicID}, nil
}

func BuildInitialPrompt(ticket Ticket) string {
	prompt := fmt.Sprintf("Work on this ticket.\n\nIssue: %s\nTitle: %s\nURL: %s", ticket.Ref(), ticket.Title, ticket.URL)
	if ticket.Description != "" {
		prompt += "\n\nDescription:\n" + ticket.Description
	}
	if ticket.AcceptanceCriteria != "" {
		prompt += "\n\nAcceptance criteria:\n" + ticket.AcceptanceCriteria
	}
	prompt += "\n\nJira workflow:\n"
	prompt += "- Use the Jira ticket as the source of truth while working. Read the ticket, comments, and linked context when needed.\n"
	prompt += "- Add concise Jira comments for meaningful progress, blockers, and important decisions.\n"
	prompt += "- When the implementation is complete and verified, move the Jira ticket to In Review.\n"
	prompt += "- If you cannot finish, leave a Jira comment with the current state, blocker, and next step."
	return prompt
}

func StartComment(ticket Ticket, entry StateEntry) string {
	message := fmt.Sprintf("CCC started work.\n\nSession: %s\nRepo: %s", entry.SessionName, entry.RepoPath)
	if entry.TopicID != 0 {
		message += fmt.Sprintf("\nTopic ID: %d", entry.TopicID)
	}
	return message
}

func StartupFailureComment(ticket Ticket, entry StateEntry) string {
	return fmt.Sprintf("CCC claimed this ticket but failed to start the session.\n\nSession: %s\nRepo: %s\nError: %s", entry.SessionName, entry.RepoPath, entry.LastError)
}
