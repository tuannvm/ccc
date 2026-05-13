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
	return prompt
}
