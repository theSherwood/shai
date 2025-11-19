package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestServerListTools(t *testing.T) {
	exec := &fakeExecutor{
		tools: []Tool{
			{Name: "alpha", Description: "first"},
			{Name: "beta", Description: "second"},
		},
	}
	server, endpoint := startTestServer(t, exec)
	defer server.Close(context.Background())

	resp := doRequest(t, endpoint, `{"jsonrpc":"2.0","id":1,"method":"listTools"}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
}

func TestServerListToolsEmpty(t *testing.T) {
	exec := &fakeExecutor{}
	server, endpoint := startTestServer(t, exec)
	defer server.Close(context.Background())

	resp := doRequest(t, endpoint, `{"jsonrpc":"2.0","id":1,"method":"listTools"}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(tools))
	}
}

func TestServerCallTool(t *testing.T) {
	exec := &fakeExecutor{
		tools: []Tool{{Name: "hello"}},
	}
	server, endpoint := startTestServer(t, exec)
	defer server.Close(context.Background())

	resp := doRequest(t, endpoint, `{"jsonrpc":"2.0","id":2,"method":"callTool","params":{"name":"hello","args":["one"]}}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	if code := int(result["exitCode"].(float64)); code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	if exec.lastName != "hello" {
		t.Fatalf("expected executor to run hello, got %s", exec.lastName)
	}
}

func TestServerRequiresAuth(t *testing.T) {
	exec := &fakeExecutor{tools: []Tool{{Name: "noop"}}}
	cfg := Config{
		Token:     "token",
		SessionID: "session",
		Executor:  exec,
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.Start()
	defer server.Close(context.Background())

	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", server.Port())
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"listTools"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func startTestServer(t *testing.T, exec Executor) (*Server, string) {
	t.Helper()
	cfg := Config{
		Token:         "secret",
		SessionID:     "session",
		Executor:      exec,
		MaxConcurrent: 1,
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.Start()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/mcp", server.Port())
	return server, endpoint
}

func doRequest(t *testing.T, endpoint, payload string) *rpcResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	var decoded rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return &decoded
}

type fakeExecutor struct {
	tools    []Tool
	lastName string
}

func (f *fakeExecutor) Tools() []Tool {
	return f.tools
}

func (f *fakeExecutor) Execute(ctx context.Context, name string, args []string, streams Streams) (int, error) {
	f.lastName = name
	if streams.Stdout != nil {
		fmt.Fprint(streams.Stdout, "ok")
	}
	select {
	case <-ctx.Done():
		return 1, ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}
	return 0, nil
}
