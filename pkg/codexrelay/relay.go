package codexrelay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/codexapp"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// AppClient is the Codex app-server surface used by the relay.
type AppClient interface {
	StartThread(context.Context, codexapp.ThreadStartParams) (*codexapp.ThreadResponse, error)
	ResumeThread(context.Context, string, codexapp.ThreadStartParams) (*codexapp.ThreadResponse, error)
	StartTurn(context.Context, string, string) (*codexapp.TurnStartResponse, error)
	StartTurnWithInput(context.Context, string, []codexapp.UserInput) (*codexapp.TurnStartResponse, error)
	InterruptTurn(context.Context, string, string) error
	Events() <-chan codexapp.Event
	Close() error
}

type ClientFactory func(context.Context) (AppClient, error)

// StreamSink receives assistant output for one Codex turn.
type StreamSink interface {
	Add(string)
	Done() (int64, error)
}

type StreamFactory func() StreamSink

// Options describes a single Telegram-to-Codex relay operation.
type Options struct {
	Config      *configpkg.Config
	SessionName string
	SessionInfo *configpkg.SessionInfo
	Prompt      string
	Input       []codexapp.UserInput
	SaveConfig  func(*configpkg.Config) error
	NewClient   ClientFactory
	NewStream   StreamFactory
	ThreadStart codexapp.ThreadStartParams
}

// TelegramStreamFactory adapts CCC's existing Telegram streaming implementation.
func TelegramStreamFactory(cfg *configpkg.Config, chatID, threadID int64) StreamFactory {
	return func() StreamSink {
		stream := telegram.NewBufferedStreamer(cfg, chatID, threadID, cfg.EnableStreaming)
		stream.Start()
		return stream
	}
}

func DefaultClientFactory(codexPath string) ClientFactory {
	return func(ctx context.Context) (AppClient, error) {
		return codexapp.StartProxy(ctx, codexPath)
	}
}

func ClientFactoryWithOptions(opts codexapp.StartOptions) ClientFactory {
	return func(ctx context.Context) (AppClient, error) {
		return codexapp.StartProxyWithOptions(ctx, opts)
	}
}

// DeliverPrompt starts or resumes a Codex app-server thread, sends a user turn,
// and forwards assistant deltas to the supplied stream sink until the turn ends.
func DeliverPrompt(ctx context.Context, opts Options) error {
	if opts.Config == nil || opts.SessionInfo == nil {
		return fmt.Errorf("missing session config")
	}
	if opts.NewClient == nil {
		opts.NewClient = DefaultClientFactory("")
	}
	if opts.NewStream == nil {
		return fmt.Errorf("missing stream factory")
	}
	if opts.SaveConfig == nil {
		opts.SaveConfig = configpkg.Save
	}

	client, err := opts.NewClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	threadID := strings.TrimSpace(opts.SessionInfo.CodexThreadID)
	params := opts.ThreadStart
	params.CWD = opts.SessionInfo.Path
	if threadID == "" {
		started, err := client.StartThread(ctx, params)
		if err != nil {
			return err
		}
		threadID = started.Thread.ID
		opts.SessionInfo.CodexThreadID = threadID
		if err := opts.SaveConfig(opts.Config); err != nil {
			return fmt.Errorf("save codex thread id: %w", err)
		}
	} else if _, err := client.ResumeThread(ctx, threadID, params); err != nil {
		return err
	}

	input := opts.Input
	if len(input) == 0 {
		input = []codexapp.UserInput{codexapp.TextInput(opts.Prompt)}
	}
	turn, err := client.StartTurnWithInput(ctx, threadID, input)
	if err != nil {
		return err
	}
	turnID := turn.Turn.ID
	opts.SessionInfo.CodexTurnID = turnID
	_ = opts.SaveConfig(opts.Config)
	defer func() {
		if opts.SessionInfo.CodexTurnID == turnID {
			opts.SessionInfo.CodexTurnID = ""
			_ = opts.SaveConfig(opts.Config)
		}
	}()

	stream := opts.NewStream()
	return forwardTurn(ctx, client.Events(), stream, threadID, turnID)
}

func Interrupt(ctx context.Context, cfg *configpkg.Config, info *configpkg.SessionInfo, newClient ClientFactory) error {
	if info == nil || info.CodexThreadID == "" || info.CodexTurnID == "" {
		return fmt.Errorf("no active Codex turn recorded for this session")
	}
	if newClient == nil {
		newClient = DefaultClientFactory("")
	}
	client, err := newClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.InterruptTurn(ctx, info.CodexThreadID, info.CodexTurnID); err != nil {
		return err
	}
	info.CodexTurnID = ""
	if cfg != nil {
		_ = configpkg.Save(cfg)
	}
	return nil
}

func forwardTurn(ctx context.Context, events <-chan codexapp.Event, stream StreamSink, threadID, turnID string) (err error) {
	completed := false
	state := relayState{}
	finalized := false
	defer func() {
		if !finalized {
			_, doneErr := stream.Done()
			if err == nil {
				err = doneErr
			}
		}
	}()
	timer := time.NewTimer(12 * time.Hour)
	defer timer.Stop()

	for !completed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("timed out waiting for Codex turn completion")
		case event, ok := <-events:
			if !ok {
				return fmt.Errorf("codex app-server stream closed before turn completed")
			}
			done, err := state.handleEvent(event, stream, threadID, turnID)
			if err != nil {
				return err
			}
			completed = done
		}
	}
	_, err = stream.Done()
	finalized = true
	return err
}

type relayState struct {
	sawAgentDelta bool
}

func (s *relayState) handleEvent(event codexapp.Event, stream StreamSink, threadID, turnID string) (bool, error) {
	switch event.Method {
	case "item/agentMessage/delta":
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Delta    string `json:"delta"`
		}
		if err := json.Unmarshal(event.Params, &params); err != nil {
			return false, err
		}
		if params.ThreadID == threadID && params.TurnID == turnID {
			s.sawAgentDelta = true
			stream.Add(params.Delta)
		}
	case "item/completed":
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(event.Params, &params); err != nil {
			return false, err
		}
		if params.ThreadID == threadID && params.TurnID == turnID && params.Item.Type == "agentMessage" && params.Item.Text != "" && !s.sawAgentDelta {
			// Delta notifications are preferred. This fallback covers app-server
			// versions that only emit completed agent messages.
			stream.Add(params.Item.Text)
		}
	case "turn/completed":
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if err := json.Unmarshal(event.Params, &params); err != nil {
			return false, err
		}
		return params.ThreadID == threadID && params.Turn.ID == turnID, nil
	case "error":
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Message  string `json:"message"`
			Error    struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(event.Params, &params); err == nil {
			if params.ThreadID != "" && params.ThreadID != threadID {
				return false, nil
			}
			if params.TurnID != "" && params.TurnID != turnID {
				return false, nil
			}
			message := params.Message
			if message == "" {
				message = params.Error.Message
			}
			if message != "" {
				return false, fmt.Errorf("codex app-server: %s", message)
			}
		}
	}
	return false, nil
}
