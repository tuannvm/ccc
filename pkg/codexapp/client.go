package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client talks to the experimental Codex app-server JSON-RPC protocol.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	enc    *json.Encoder

	nextID  atomic.Int64
	writeMu sync.Mutex
	mu      sync.Mutex
	closeMu sync.Once
	pending map[int64]chan response
	events  chan Event
	closed  chan struct{}
}

type response struct {
	Result json.RawMessage
	Err    *RPCError
}

// RPCError is a JSON-RPC error returned by app-server.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("codex app-server error %d: %s", e.Code, e.Message)
}

type envelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Event is a server notification emitted by app-server.
type Event struct {
	ID     *int64
	Method string
	Params json.RawMessage
}

// StartProxy starts a Codex app-server stdio transport and initializes it.
func StartProxy(ctx context.Context, codexPath string) (*Client, error) {
	return StartProxyWithOptions(ctx, StartOptions{CodexPath: codexPath})
}

type StartOptions struct {
	CodexPath  string
	Env        []string
	ConfigArgs []string
}

func StartProxyWithOptions(ctx context.Context, opts StartOptions) (*Client, error) {
	codexPath := opts.CodexPath
	if codexPath == "" {
		codexPath = "codex"
	}
	args := append([]string{"app-server"}, opts.ConfigArgs...)
	args = append(args, "--listen", "stdio://")
	cmd := exec.CommandContext(ctx, codexPath, args...)
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := NewClient(stdin, stdout)
	c.cmd = cmd
	go c.readLoop()
	title := "CCC Codex Relay"
	if _, err := c.Initialize(ctx, ClientInfo{Name: "ccc", Title: &title, Version: "1"}); err != nil {
		_ = c.Close()
		return nil, err
	}
	if err := c.Initialized(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// NewClient creates a client around an already connected app-server stream.
func NewClient(stdin io.WriteCloser, stdout io.ReadCloser) *Client {
	return &Client{
		stdin:   stdin,
		stdout:  stdout,
		enc:     json.NewEncoder(stdin),
		pending: make(map[int64]chan response),
		events:  make(chan Event, 128),
		closed:  make(chan struct{}),
	}
}

func (c *Client) Events() <-chan Event {
	return c.events
}

func (c *Client) Close() error {
	c.closeWithError(io.ErrClosedPipe)
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.stdout != nil {
		_ = c.stdout.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	return nil
}

func (c *Client) closeWithError(err error) {
	c.closeMu.Do(func() {
		c.mu.Lock()
		for id, ch := range c.pending {
			delete(c.pending, id)
			ch <- response{Err: &RPCError{Code: -32000, Message: err.Error()}}
		}
		c.mu.Unlock()
		close(c.closed)
	})
}

func (c *Client) request(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ch := make(chan response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	c.writeMu.Lock()
	err = c.enc.Encode(envelope{JSONRPC: "2.0", ID: &id, Method: method, Params: paramsRaw})
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(id)
		return err
	}

	select {
	case resp := <-ch:
		if resp.Err != nil {
			return resp.Err
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(resp.Result, out)
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.closed:
		c.removePending(id)
		return io.ErrClosedPipe
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	var paramsRaw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		paramsRaw = data
	}
	c.writeMu.Lock()
	err := c.enc.Encode(envelope{JSONRPC: "2.0", Method: method, Params: paramsRaw})
	c.writeMu.Unlock()
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (c *Client) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) readLoop() {
	defer close(c.events)
	dec := json.NewDecoder(bufio.NewReader(c.stdout))
	for {
		var msg envelope
		if err := dec.Decode(&msg); err != nil {
			c.closeWithError(err)
			return
		}
		if msg.Method != "" {
			if msg.ID != nil {
				c.respondError(*msg.ID, -32601, "ccc Codex relay does not support app-server server requests yet")
			}
			select {
			case c.events <- Event{ID: msg.ID, Method: msg.Method, Params: msg.Params}:
			case <-c.closed:
				return
			}
			continue
		}
		if msg.ID != nil {
			c.mu.Lock()
			ch := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- response{Result: msg.Result, Err: msg.Error}
			}
			continue
		}
	}
}

func (c *Client) respondError(id int64, code int, message string) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.enc.Encode(envelope{
		JSONRPC: "2.0",
		ID:      &id,
		Error:   &RPCError{Code: code, Message: message},
	})
}

type ClientInfo struct {
	Name    string  `json:"name"`
	Title   *string `json:"title"`
	Version string  `json:"version"`
}

func (c *Client) Initialize(ctx context.Context, info ClientInfo) (*InitializeResponse, error) {
	var out InitializeResponse
	err := c.request(ctx, "initialize", map[string]any{
		"clientInfo": info,
		"capabilities": map[string]any{
			"experimentalApi":    false,
			"requestAttestation": false,
		},
	}, &out)
	return &out, err
}

func (c *Client) Initialized(ctx context.Context) error {
	return c.notify(ctx, "initialized", nil)
}

type InitializeResponse struct {
	UserAgent      string `json:"userAgent"`
	CodexHome      string `json:"codexHome"`
	PlatformFamily string `json:"platformFamily"`
	PlatformOS     string `json:"platformOs"`
}

type Thread struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Preview   string `json:"preview"`
	CWD       string `json:"cwd"`
	Status    any    `json:"status"`
}

type ThreadResponse struct {
	Thread Thread `json:"thread"`
}

type ThreadStartParams struct {
	CWD           string         `json:"cwd,omitempty"`
	Model         string         `json:"model,omitempty"`
	ModelProvider string         `json:"modelProvider,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
}

func (c *Client) StartThread(ctx context.Context, params ThreadStartParams) (*ThreadResponse, error) {
	var out ThreadResponse
	err := c.request(ctx, "thread/start", params, &out)
	return &out, err
}

func (c *Client) ResumeThread(ctx context.Context, threadID string, params ThreadStartParams) (*ThreadResponse, error) {
	var out ThreadResponse
	err := c.request(ctx, "thread/resume", ThreadResumeParams{
		ThreadID:      threadID,
		CWD:           params.CWD,
		Model:         params.Model,
		ModelProvider: params.ModelProvider,
		Config:        params.Config,
	}, &out)
	return &out, err
}

type ThreadResumeParams struct {
	ThreadID      string         `json:"threadId"`
	CWD           string         `json:"cwd,omitempty"`
	Model         string         `json:"model,omitempty"`
	ModelProvider string         `json:"modelProvider,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
}

type Turn struct {
	ID string `json:"id"`
}

type TurnStartResponse struct {
	Turn Turn `json:"turn"`
}

type UserInput struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	TextElements []any  `json:"text_elements,omitempty"`
	Path         string `json:"path,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

func TextInput(text string) UserInput {
	return UserInput{Type: "text", Text: text, TextElements: []any{}}
}

func LocalImageInput(path string) UserInput {
	return UserInput{Type: "localImage", Path: path}
}

func (c *Client) StartTurn(ctx context.Context, threadID string, text string) (*TurnStartResponse, error) {
	return c.StartTurnWithInput(ctx, threadID, []UserInput{TextInput(text)})
}

func (c *Client) StartTurnWithInput(ctx context.Context, threadID string, input []UserInput) (*TurnStartResponse, error) {
	var out TurnStartResponse
	err := c.request(ctx, "turn/start", map[string]any{
		"threadId": threadID,
		"input":    input,
	}, &out)
	return &out, err
}

func (c *Client) InterruptTurn(ctx context.Context, threadID, turnID string) error {
	return c.request(ctx, "turn/interrupt", map[string]any{"threadId": threadID, "turnId": turnID}, nil)
}
