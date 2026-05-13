package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	jira "github.com/andygrunwald/go-jira/v2/onpremise"
)

type bearerTransport struct {
	method string
	token  string
	email  string
	base   http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if t.method == "basic" {
		req.SetBasicAuth(t.email, t.token)
	} else {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

type JiraProvider struct {
	Config     JiraConfig
	Client     *jira.Client
	baseURL    string
	httpClient *http.Client
}

func NewJiraProvider(cfg JiraConfig) (*JiraProvider, error) {
	method := cfg.AuthMethod
	if method == "" {
		if cfg.AuthEmail != "" {
			method = "basic"
		} else {
			method = "bearer"
		}
	}
	hc := &http.Client{Transport: bearerTransport{method: method, token: cfg.AuthToken, email: cfg.AuthEmail}}
	client, err := jira.NewClient(strings.TrimRight(cfg.BaseURL, "/"), hc)
	if err != nil {
		return nil, err
	}
	return &JiraProvider{Config: cfg, Client: client, baseURL: strings.TrimRight(cfg.BaseURL, "/"), httpClient: hc}, nil
}

func (p *JiraProvider) Name() string {
	return "jira"
}

func (p *JiraProvider) PollCandidates(ctx context.Context, max int) ([]Ticket, error) {
	if max <= 0 {
		max = 1
	}
	fields := []string{"summary", "description", p.Config.RepoField}
	if p.Config.AcceptanceCriteriaField != "" {
		fields = append(fields, p.Config.AcceptanceCriteriaField)
	}
	issues, err := p.searchJQL(ctx, p.Config.JQL, fields, max)
	if err != nil {
		return nil, err
	}
	tickets := make([]Ticket, 0, len(issues))
	for _, issue := range issues {
		ticket, err := p.ticketFromRawIssue(issue)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}
	return tickets, nil
}

type jiraSearchResponse struct {
	Issues []jiraRawIssue `json:"issues"`
}

type jiraRawIssue struct {
	ID     string                     `json:"id"`
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

func (p *JiraProvider) searchJQL(ctx context.Context, jql string, fields []string, max int) ([]jiraRawIssue, error) {
	endpoint, err := url.Parse(p.baseURL + "/rest/api/3/search/jql")
	if err != nil {
		return nil, err
	}
	q := endpoint.Query()
	q.Set("jql", jql)
	q.Set("maxResults", fmt.Sprint(max))
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return nil, fmt.Errorf("jira search failed: status %d: %v", resp.StatusCode, body)
	}
	var result jiraSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Issues, nil
}

func (p *JiraProvider) Claim(ctx context.Context, ticket Ticket) error {
	transitions, _, err := p.Client.Issue.GetTransitions(ctx, ticket.Ref())
	if err != nil {
		return err
	}
	transitionID := ""
	wantedTransition := strings.TrimSpace(p.Config.ClaimTransition)
	wantedStatus := strings.TrimSpace(p.Config.ClaimStatus)
	for _, tr := range transitions {
		if wantedTransition != "" && (tr.ID == wantedTransition || strings.EqualFold(tr.Name, wantedTransition)) {
			transitionID = tr.ID
			break
		}
		if wantedStatus != "" && strings.EqualFold(tr.To.Name, wantedStatus) {
			transitionID = tr.ID
			break
		}
	}
	if transitionID == "" {
		return fmt.Errorf("no Jira transition found for claim_transition=%q claim_status=%q", wantedTransition, wantedStatus)
	}
	_, err = p.Client.Issue.DoTransition(ctx, ticket.Ref(), transitionID)
	return err
}

func (p *JiraProvider) FetchContext(ctx context.Context, ticket Ticket) (Ticket, error) {
	fields := []string{"summary", "description", p.Config.RepoField}
	if p.Config.AcceptanceCriteriaField != "" {
		fields = append(fields, p.Config.AcceptanceCriteriaField)
	}
	issue, err := p.getIssue(ctx, ticket.Ref(), fields)
	if err != nil {
		return Ticket{}, err
	}
	return p.ticketFromRawIssue(issue)
}

func (p *JiraProvider) Comment(ctx context.Context, ticket Ticket, message string) error {
	_, _, err := p.Client.Issue.AddComment(ctx, ticket.Ref(), &jira.Comment{Body: message})
	return err
}

func (p *JiraProvider) getIssue(ctx context.Context, key string, fields []string) (jiraRawIssue, error) {
	endpoint, err := url.Parse(p.baseURL + "/rest/api/3/issue/" + url.PathEscape(key))
	if err != nil {
		return jiraRawIssue{}, err
	}
	q := endpoint.Query()
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return jiraRawIssue{}, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return jiraRawIssue{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return jiraRawIssue{}, fmt.Errorf("jira issue get failed: status %d: %v", resp.StatusCode, body)
	}
	var issue jiraRawIssue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return jiraRawIssue{}, err
	}
	return issue, nil
}

func (p *JiraProvider) ticketFromRawIssue(issue jiraRawIssue) (Ticket, error) {
	if issue.Fields == nil {
		return Ticket{}, fmt.Errorf("%s has no fields in Jira response", issue.Key)
	}
	repoRef := strings.TrimSpace(rawFieldAsString(issue.Fields[p.Config.RepoField]))
	if repoRef == "" && p.Config.RepoFallback != "" {
		repoRef = p.Config.RepoFallback
	}
	t := Ticket{
		ID:          issue.ID,
		Key:         issue.Key,
		Title:       rawFieldAsString(issue.Fields["summary"]),
		Description: rawFieldAsString(issue.Fields["description"]),
		URL:         strings.TrimRight(p.Config.BaseURL, "/") + "/browse/" + issue.Key,
		RepoRef:     repoRef,
		Provider:    p.Name(),
	}
	if p.Config.AcceptanceCriteriaField != "" {
		t.AcceptanceCriteria = rawFieldAsString(issue.Fields[p.Config.AcceptanceCriteriaField])
	}
	return t, nil
}

func rawFieldAsString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return valueAsString(value)
}

func valueAsString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := strings.TrimSpace(valueAsString(item)); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"value", "name", "displayName", "key"} {
			if s := strings.TrimSpace(valueAsString(x[key])); s != "" {
				return s
			}
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}
