package codexapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestClientStartTurnAndNotification(t *testing.T) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	defer clientToServerR.Close()
	defer clientToServerW.Close()
	defer serverToClientR.Close()
	defer serverToClientW.Close()

	client := NewClient(clientToServerW, serverToClientR)
	go client.readLoop()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		dec := json.NewDecoder(clientToServerR)
		enc := json.NewEncoder(serverToClientW)
		var req envelope
		if err := dec.Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req.Method != "turn/start" {
			t.Errorf("method = %q, want turn/start", req.Method)
			return
		}
		if req.ID == nil {
			t.Error("request id is nil")
			return
		}
		if err := enc.Encode(envelope{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"turn":{"id":"turn-1"}}`),
		}); err != nil {
			t.Errorf("encode response: %v", err)
			return
		}
		_ = enc.Encode(envelope{
			JSONRPC: "2.0",
			Method:  "item/agentMessage/delta",
			Params:  json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","delta":"hello"}`),
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	turn, err := client.StartTurn(ctx, "thread-1", "prompt")
	if err != nil {
		t.Fatalf("StartTurn error: %v", err)
	}
	if turn.Turn.ID != "turn-1" {
		t.Fatalf("turn id = %q, want turn-1", turn.Turn.ID)
	}

	select {
	case event := <-client.Events():
		if event.Method != "item/agentMessage/delta" {
			t.Fatalf("event method = %q", event.Method)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}

	<-serverDone
}

func TestClientRequestUnblocksWhenTransportCloses(t *testing.T) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	defer clientToServerR.Close()
	defer clientToServerW.Close()
	defer serverToClientR.Close()

	client := NewClient(clientToServerW, serverToClientR)
	go client.readLoop()
	go func() {
		dec := json.NewDecoder(clientToServerR)
		var req envelope
		_ = dec.Decode(&req)
		_ = serverToClientW.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.StartTurn(ctx, "thread-1", "prompt")
	if err == nil {
		t.Fatal("StartTurn error = nil, want transport close error")
	}
	if !strings.Contains(err.Error(), "io: read/write on closed pipe") && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("StartTurn error = %q, want closed transport error", err.Error())
	}
}

func TestClientInitializedNotification(t *testing.T) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	defer clientToServerR.Close()
	defer clientToServerW.Close()
	defer serverToClientR.Close()
	defer serverToClientW.Close()

	client := NewClient(clientToServerW, serverToClientR)
	serverErr := make(chan error, 1)
	go func() {
		defer close(serverErr)
		dec := json.NewDecoder(clientToServerR)
		var msg envelope
		if err := dec.Decode(&msg); err != nil {
			serverErr <- fmt.Errorf("decode notification: %w", err)
			return
		}
		if msg.ID != nil {
			serverErr <- fmt.Errorf("initialized notification id = %v, want nil", *msg.ID)
			return
		}
		if msg.Method != "initialized" {
			serverErr <- fmt.Errorf("method = %q, want initialized", msg.Method)
			return
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Initialized(ctx); err != nil {
		t.Fatalf("Initialized error: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsUnsupportedServerRequest(t *testing.T) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	defer clientToServerR.Close()
	defer clientToServerW.Close()
	defer serverToClientR.Close()
	defer serverToClientW.Close()

	client := NewClient(clientToServerW, serverToClientR)
	go client.readLoop()

	serverErr := make(chan error, 1)
	go func() {
		defer close(serverErr)
		enc := json.NewEncoder(serverToClientW)
		dec := json.NewDecoder(clientToServerR)
		id := int64(99)
		if err := enc.Encode(envelope{
			JSONRPC: "2.0",
			ID:      &id,
			Method:  "item/permissions/requestApproval",
			Params:  json.RawMessage("{\"threadId\":\"thread-1\"}"),
		}); err != nil {
			serverErr <- fmt.Errorf("encode server request: %w", err)
			return
		}
		var resp envelope
		if err := dec.Decode(&resp); err != nil {
			serverErr <- fmt.Errorf("decode rejection: %w", err)
			return
		}
		if resp.ID == nil || *resp.ID != id {
			serverErr <- fmt.Errorf("response id = %v, want %d", resp.ID, id)
			return
		}
		if resp.Error == nil || resp.Error.Code != -32601 {
			serverErr <- fmt.Errorf("response error = %+v, want -32601", resp.Error)
			return
		}
	}()

	select {
	case event := <-client.Events():
		if event.Method != "item/permissions/requestApproval" {
			t.Fatalf("event method = %q", event.Method)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server request event")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
