package watch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJiraProviderPollsWithJQLAndExtractsRepoField(t *testing.T) {
	var gotJQL, gotFields, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotJQL = r.URL.Query().Get("jql")
		gotFields = r.URL.Query().Get("fields")
		writeJSON(t, w, map[string]any{
			"issues": []any{map[string]any{
				"id":  "10001",
				"key": "ABC-1",
				"fields": map[string]any{
					"summary":           "Do work",
					"description":       "Details",
					"customfield_10001": "git@github.com:liftoff/ccc.git",
				},
			}},
		})
	}))
	defer server.Close()

	provider, err := NewJiraProvider(JiraConfig{
		BaseURL:     server.URL,
		AuthToken:   "secret",
		JQL:         "project = ABC",
		RepoField:   "customfield_10001",
		ClaimStatus: "In Progress",
	})
	if err != nil {
		t.Fatal(err)
	}
	tickets, err := provider.PollCandidates(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotJQL != "project = ABC" {
		t.Fatalf("jql = %q", gotJQL)
	}
	if !strings.Contains(gotFields, "customfield_10001") {
		t.Fatalf("fields = %q", gotFields)
	}
	if len(tickets) != 1 || tickets[0].RepoRef != "git@github.com:liftoff/ccc.git" || tickets[0].Key != "ABC-1" {
		t.Fatalf("tickets = %+v", tickets)
	}
}

func TestJiraProviderUsesBasicAuthWhenEmailConfigured(t *testing.T) {
	var gotUser, gotPass string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		writeJSON(t, w, map[string]any{"issues": []any{}})
	}))
	defer server.Close()

	provider, err := NewJiraProvider(JiraConfig{
		BaseURL:    server.URL,
		AuthMethod: "basic",
		AuthEmail:  "user@example.com",
		AuthToken:  "api-key",
		JQL:        "project = ABC",
		RepoField:  "customfield_10001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.PollCandidates(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if gotUser != "user@example.com" || gotPass != "api-key" {
		t.Fatalf("basic auth = %q:%q", gotUser, gotPass)
	}
}

func TestJiraProviderSearchReportsRESTFallbackErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		http.Error(w, `{"errorMessages":["bad jql"]}`, http.StatusBadRequest)
	}))
	defer server.Close()

	provider, err := NewJiraProvider(JiraConfig{
		BaseURL:   server.URL,
		AuthToken: "secret",
		JQL:       "bad",
		RepoField: "customfield_10001",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.PollCandidates(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "jira search failed: status 400") {
		t.Fatalf("err = %v", err)
	}
}

func TestJiraProviderClaimTransitionsByStatus(t *testing.T) {
	var posted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/ABC-2/transitions":
			writeJSON(t, w, map[string]any{"transitions": []any{
				map[string]any{"id": "11", "name": "Start Progress", "to": map[string]any{"name": "In Progress"}},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/2/issue/ABC-2/transitions":
			posted = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			transition := payload["transition"].(map[string]any)
			if transition["id"] != "11" {
				t.Fatalf("transition id = %v", transition["id"])
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := NewJiraProvider(JiraConfig{BaseURL: server.URL, AuthToken: "secret", ClaimStatus: "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Claim(context.Background(), Ticket{Key: "ABC-2"}); err != nil {
		t.Fatal(err)
	}
	if !posted {
		t.Fatal("transition was not posted")
	}
}

func TestJiraProviderUsesRepoFallbackWhenFieldIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"issues": []any{
			map[string]any{"id": "1", "key": "ABC-3", "fields": map[string]any{"summary": "Missing repo"}},
		}})
	}))
	defer server.Close()
	provider, err := NewJiraProvider(JiraConfig{BaseURL: server.URL, AuthToken: "secret", JQL: "project = ABC", RepoField: "customfield_10001", RepoFallback: "/tmp/fallback", ClaimStatus: "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	tickets, err := provider.PollCandidates(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 1 || tickets[0].RepoRef != "/tmp/fallback" {
		t.Fatalf("tickets = %+v", tickets)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}
