package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Tool describes a single alias published via MCP.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Streams configures stdout/stderr writers for an execution.
type Streams struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Executor defines the interface the MCP server uses to run alias commands.
type Executor interface {
	Tools() []Tool
	Execute(ctx context.Context, name string, args []string, streams Streams) (int, error)
}

// Logger emits debug messages from the server.
type Logger interface {
	Printf(format string, args ...interface{})
}

// Config supplies the inputs required to start the MCP alias server.
type Config struct {
	BindAddr      string
	Token         string
	SessionID     string
	Executor      Executor
	Logger        Logger
	MaxConcurrent int
}

// Server hosts alias commands as MCP tools.
type Server struct {
	cfg        Config
	httpServer *http.Server
	listener   net.Listener
	entryMap   map[string]Tool
	tools      []toolDescriptor
	sem        chan struct{}
	logger     Logger
	executor   Executor

	mu    sync.RWMutex
	alive bool
}

// Tool metadata presented via listTools.
type toolDescriptor struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// OutputChunk carries command output in MCP format.
type OutputChunk struct {
	Type   string `json:"type"`
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

// CallResult models the MCP response payload.
type CallResult struct {
	ExitCode int           `json:"exitCode"`
	Content  []OutputChunk `json:"content"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewServer constructs an MCP alias server bound to the provided address.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Executor == nil {
		return nil, fmt.Errorf("executor is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("auth token is required")
	}
	if cfg.SessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	bindAddr := strings.TrimSpace(cfg.BindAddr)
	if bindAddr == "" {
		bindAddr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", bindAddr, err)
	}

	entryMap := make(map[string]Tool)
	executorTools := cfg.Executor.Tools()
	tools := make([]toolDescriptor, 0, len(executorTools))
	for _, tool := range executorTools {
		if tool.Name == "" {
			continue
		}
		entryMap[tool.Name] = tool
		desc := tool.Description
		if strings.TrimSpace(desc) == "" {
			desc = fmt.Sprintf("Runs alias %s on the host", tool.Name)
		}
		tools = append(tools, toolDescriptor{
			Name:        tool.Name,
			Description: desc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"args": map[string]interface{}{
						"type":  "array",
						"items": map[string]string{"type": "string"},
					},
				},
			},
		})
	}

	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}

	server := &Server{
		cfg:      cfg,
		listener: ln,
		entryMap: entryMap,
		tools:    tools,
		sem:      make(chan struct{}, maxConcurrent),
		logger:   cfg.Logger,
		executor: cfg.Executor,
		alive:    true,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", server.handleRPC)
	server.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return server, nil
}

// Port returns the bound TCP port.
func (s *Server) Port() int {
	if s.listener == nil {
		return 0
	}
	if addr, ok := s.listener.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// Start begins serving requests in the background.
func (s *Server) Start() {
	go func() {
		if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logf("alias MCP server stopped: %v", err)
		}
	}()
}

// Close shuts down the HTTP server gracefully.
func (s *Server) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alive {
		return nil
	}
	s.alive = false
	if ctx == nil {
		ctx = context.Background()
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	switch req.Method {
	case "listTools":
		s.writeResponse(w, rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": s.tools,
			},
		})
	case "callTool":
		resp := s.handleCallTool(r.Context(), req)
		s.writeResponse(w, resp)
	default:
		s.writeResponse(w, rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32601,
				Message: "method not found",
			},
		})
	}
}

func (s *Server) handleCallTool(ctx context.Context, req rpcRequest) rpcResponse {
	var params struct {
		Name string   `json:"name"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32602,
				Message: fmt.Sprintf("invalid params: %v", err),
			},
		}
	}
	tool, ok := s.entryMap[params.Name]
	if !ok {
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32001,
				Message: fmt.Sprintf("alias %q not found", params.Name),
			},
		}
	}

	select {
	case s.sem <- struct{}{}:
	default:
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32002,
				Message: "alias execution pool exhausted",
			},
		}
	}
	defer func() { <-s.sem }()

	collector := newOutputCollector()
	exitCode, err := s.executor.Execute(ctx, tool.Name, params.Args, Streams{
		Stdout: collector.writer("stdout"),
		Stderr: collector.writer("stderr"),
	})
	if err != nil {
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32003,
				Message: err.Error(),
			},
		}
	}

	return rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: CallResult{
			ExitCode: exitCode,
			Content:  collector.chunks(),
		},
	}
}

func (s *Server) writeResponse(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(resp)
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if token != s.cfg.Token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) logf(format string, args ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Printf(format, args...)
}

type outputCollector struct {
	mu     sync.Mutex
	values []OutputChunk
}

func newOutputCollector() *outputCollector {
	return &outputCollector{
		values: make([]OutputChunk, 0, 8),
	}
}

func (c *outputCollector) writer(stream string) io.Writer {
	return writerFunc(func(p []byte) (int, error) {
		if len(p) == 0 {
			return 0, nil
		}
		text := string(p)
		c.mu.Lock()
		c.values = append(c.values, OutputChunk{
			Type:   "text",
			Stream: stream,
			Text:   text,
		})
		c.mu.Unlock()
		return len(p), nil
	})
}

func (c *outputCollector) chunks() []OutputChunk {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]OutputChunk, len(c.values))
	copy(out, c.values)
	return out
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}
