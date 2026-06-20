package codexrelay

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tuannvm/ccc/pkg/codexapp"
	configpkg "github.com/tuannvm/ccc/pkg/config"
)

type fakeClient struct {
	events       chan codexapp.Event
	started      bool
	resumed      bool
	turnID       string
	threadID     string
	interrupt    bool
	startParams  codexapp.ThreadStartParams
	resumeParams codexapp.ThreadStartParams
}

func (f *fakeClient) StartThread(_ context.Context, params codexapp.ThreadStartParams) (*codexapp.ThreadResponse, error) {
	f.started = true
	f.startParams = params
	return &codexapp.ThreadResponse{Thread: codexapp.Thread{ID: f.threadID}}, nil
}

func (f *fakeClient) ResumeThread(_ context.Context, _ string, params codexapp.ThreadStartParams) (*codexapp.ThreadResponse, error) {
	f.resumed = true
	f.resumeParams = params
	return &codexapp.ThreadResponse{Thread: codexapp.Thread{ID: f.threadID}}, nil
}

func (f *fakeClient) StartTurn(context.Context, string, string) (*codexapp.TurnStartResponse, error) {
	return &codexapp.TurnStartResponse{Turn: codexapp.Turn{ID: f.turnID}}, nil
}

func (f *fakeClient) StartTurnWithInput(context.Context, string, []codexapp.UserInput) (*codexapp.TurnStartResponse, error) {
	return &codexapp.TurnStartResponse{Turn: codexapp.Turn{ID: f.turnID}}, nil
}

func (f *fakeClient) InterruptTurn(context.Context, string, string) error {
	f.interrupt = true
	return nil
}

func (f *fakeClient) Events() <-chan codexapp.Event { return f.events }
func (f *fakeClient) Close() error                  { return nil }

type captureStream struct {
	text string
	done bool
}

func (s *captureStream) Add(text string) {
	s.text += text
}

func (s *captureStream) Done() (int64, error) {
	s.done = true
	return 1, nil
}

func event(method string, params string) codexapp.Event {
	return codexapp.Event{Method: method, Params: json.RawMessage(params)}
}

func TestDeliverPromptStreamsDeltasWithoutCompletedDuplicate(t *testing.T) {
	fake := &fakeClient{
		events:   make(chan codexapp.Event, 4),
		threadID: "thread-1",
		turnID:   "turn-1",
	}
	fake.events <- event("item/agentMessage/delta", `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"hel"}`)
	fake.events <- event("item/agentMessage/delta", `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"lo"}`)
	fake.events <- event("item/completed", `{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"item-1","text":"hello"}}`)
	fake.events <- event("turn/completed", `{"threadId":"thread-1","turn":{"id":"turn-1"}}`)

	stream := &captureStream{}
	cfg := &configpkg.Config{Sessions: map[string]*configpkg.SessionInfo{}}
	info := &configpkg.SessionInfo{Path: "/tmp/project"}
	cfg.Sessions["project"] = info

	err := DeliverPrompt(context.Background(), Options{
		Config:      cfg,
		SessionName: "project",
		SessionInfo: info,
		Prompt:      "say hello",
		NewClient: func(context.Context) (AppClient, error) {
			return fake, nil
		},
		NewStream: func() StreamSink {
			return stream
		},
		ThreadStart: codexapp.ThreadStartParams{
			Model:         "model-a",
			ModelProvider: "provider-a",
			Config:        map[string]any{"model_provider": "provider-a"},
		},
		SaveConfig: func(*configpkg.Config) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("DeliverPrompt error: %v", err)
	}
	if !fake.started {
		t.Fatal("StartThread was not called")
	}
	if info.CodexThreadID != "thread-1" {
		t.Fatalf("CodexThreadID = %q, want thread-1", info.CodexThreadID)
	}
	if fake.startParams.Model != "model-a" || fake.startParams.ModelProvider != "provider-a" || fake.startParams.Config["model_provider"] != "provider-a" {
		t.Fatalf("start params = %+v, want provider thread overrides", fake.startParams)
	}
	if info.CodexTurnID != "" {
		t.Fatalf("CodexTurnID = %q, want cleared after completion", info.CodexTurnID)
	}
	if stream.text != "hello" {
		t.Fatalf("stream text = %q, want hello", stream.text)
	}
	if strings.Count(stream.text, "hello") != 1 {
		t.Fatalf("stream duplicated completed text: %q", stream.text)
	}
	if !stream.done {
		t.Fatal("stream was not finalized")
	}
}

func TestDeliverPromptPassesProviderOverridesWhenResumingThread(t *testing.T) {
	fake := &fakeClient{
		events:   make(chan codexapp.Event, 1),
		threadID: "thread-1",
		turnID:   "turn-1",
	}
	fake.events <- event("turn/completed", `{"threadId":"thread-1","turn":{"id":"turn-1"}}`)
	stream := &captureStream{}
	cfg := &configpkg.Config{Sessions: map[string]*configpkg.SessionInfo{}}
	info := &configpkg.SessionInfo{Path: "/tmp/project", CodexThreadID: "thread-1"}
	cfg.Sessions["project"] = info

	err := DeliverPrompt(context.Background(), Options{
		Config:      cfg,
		SessionName: "project",
		SessionInfo: info,
		Prompt:      "say hello",
		NewClient: func(context.Context) (AppClient, error) {
			return fake, nil
		},
		NewStream: func() StreamSink {
			return stream
		},
		ThreadStart: codexapp.ThreadStartParams{
			Model:         "model-a",
			ModelProvider: "provider-a",
			Config:        map[string]any{"model_provider": "provider-a"},
		},
		SaveConfig: func(*configpkg.Config) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("DeliverPrompt error: %v", err)
	}
	if fake.started {
		t.Fatal("StartThread was called for existing thread")
	}
	if !fake.resumed {
		t.Fatal("ResumeThread was not called")
	}
	if fake.resumeParams.Model != "model-a" || fake.resumeParams.ModelProvider != "provider-a" || fake.resumeParams.Config["model_provider"] != "provider-a" {
		t.Fatalf("resume params = %+v, want provider thread overrides", fake.resumeParams)
	}
}

func TestDeliverPromptReturnsErrorWhenStreamClosesBeforeTurnCompletes(t *testing.T) {
	fake := &fakeClient{
		events:   make(chan codexapp.Event),
		threadID: "thread-1",
		turnID:   "turn-1",
	}
	close(fake.events)
	stream := &captureStream{}
	cfg := &configpkg.Config{Sessions: map[string]*configpkg.SessionInfo{}}
	info := &configpkg.SessionInfo{Path: "/tmp/project"}
	cfg.Sessions["project"] = info

	err := DeliverPrompt(context.Background(), Options{
		Config:      cfg,
		SessionName: "project",
		SessionInfo: info,
		Prompt:      "say hello",
		NewClient: func(context.Context) (AppClient, error) {
			return fake, nil
		},
		NewStream: func() StreamSink {
			return stream
		},
		SaveConfig: func(*configpkg.Config) error {
			return nil
		},
	})
	if err == nil {
		t.Fatal("DeliverPrompt error = nil, want early close error")
	}
	if !strings.Contains(err.Error(), "stream closed before turn completed") {
		t.Fatalf("DeliverPrompt error = %q, want stream closed error", err.Error())
	}
	if !stream.done {
		t.Fatal("stream should finalize on early close")
	}
}

func TestDeliverPromptReturnsNestedErrorNotificationMessage(t *testing.T) {
	fake := &fakeClient{
		events:   make(chan codexapp.Event, 1),
		threadID: "thread-1",
		turnID:   "turn-1",
	}
	fake.events <- event("error", "{\"threadId\":\"thread-1\",\"turnId\":\"turn-1\",\"error\":{\"message\":\"permission denied\"},\"willRetry\":false}")
	stream := &captureStream{}
	cfg := &configpkg.Config{Sessions: map[string]*configpkg.SessionInfo{}}
	info := &configpkg.SessionInfo{Path: "/tmp/project"}
	cfg.Sessions["project"] = info

	err := DeliverPrompt(context.Background(), Options{
		Config:      cfg,
		SessionName: "project",
		SessionInfo: info,
		Prompt:      "say hello",
		NewClient: func(context.Context) (AppClient, error) {
			return fake, nil
		},
		NewStream: func() StreamSink {
			return stream
		},
		SaveConfig: func(*configpkg.Config) error {
			return nil
		},
	})
	if err == nil {
		t.Fatal("DeliverPrompt error = nil, want error notification")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("DeliverPrompt error = %q, want permission denied", err.Error())
	}
}

func TestDeliverPromptIgnoresErrorNotificationForOtherTurn(t *testing.T) {
	fake := &fakeClient{
		events:   make(chan codexapp.Event, 2),
		threadID: "thread-1",
		turnID:   "turn-1",
	}
	fake.events <- event("error", "{\"threadId\":\"thread-2\",\"turnId\":\"turn-2\",\"error\":{\"message\":\"other failed\"},\"willRetry\":false}")
	fake.events <- event("turn/completed", "{\"threadId\":\"thread-1\",\"turn\":{\"id\":\"turn-1\"}}")
	stream := &captureStream{}
	cfg := &configpkg.Config{Sessions: map[string]*configpkg.SessionInfo{}}
	info := &configpkg.SessionInfo{Path: "/tmp/project"}
	cfg.Sessions["project"] = info

	err := DeliverPrompt(context.Background(), Options{
		Config:      cfg,
		SessionName: "project",
		SessionInfo: info,
		Prompt:      "say hello",
		NewClient: func(context.Context) (AppClient, error) {
			return fake, nil
		},
		NewStream: func() StreamSink {
			return stream
		},
		SaveConfig: func(*configpkg.Config) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("DeliverPrompt error = %v, want unrelated error ignored", err)
	}
}
