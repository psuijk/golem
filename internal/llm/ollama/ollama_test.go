package ollama_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/psuijk/golem/internal/llm"
	"github.com/psuijk/golem/internal/llm/ollama"
)

func floatPtr(f float32) *float32 { return &f }

func TestStreamLive(t *testing.T) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		t.Skip("OLLAMA_MODEL not set, skipping live test")
	}

	client := ollama.New(nil, host)

	params := llm.RequestParams{
		Model:        model,
		SystemPrompt: "You are a helpful assistant. Respond in one short sentence.",
		MaxTokens:    64,
		Temperature:  floatPtr(0.0),
		Messages: []llm.Message{
			{
				Role: llm.RoleUser,
				Content: []llm.Content{
					llm.TextContent{Text: "What is 2+2?"},
				},
			},
		},
	}

	events, err := client.Stream(context.Background(), params)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var gotText bool
	var gotStop bool
	var fullText string

	for evt := range events {
		switch e := evt.(type) {
		case llm.TextDelta:
			gotText = true
			fullText += e.Text
			fmt.Print(e.Text)
		case llm.MessageStop:
			gotStop = true
			t.Logf("\n[stop] reason=%s input_tokens=%d output_tokens=%d",
				e.StopReason, e.Usage.InputTokens, e.Usage.OutputTokens)
		case llm.ErrorEvent:
			t.Fatalf("stream error: %v", e.Err)
		}
	}

	fmt.Println()

	if !gotText {
		t.Error("no TextDelta events received")
	}
	if !gotStop {
		t.Error("no MessageStop event received")
	}
	if fullText == "" {
		t.Error("fullText is empty")
	}

	t.Logf("full response: %q", fullText)
}

func TestStreamWithTools(t *testing.T) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		t.Skip("OLLAMA_MODEL not set, skipping live test")
	}

	client := ollama.New(nil, host)

	params := llm.RequestParams{
		Model:        model,
		SystemPrompt: "You are a helpful assistant. When asked to read a file, always use the readfile tool.",
		MaxTokens:    256,
		Temperature:  floatPtr(0.0),
		Messages: []llm.Message{
			{
				Role: llm.RoleUser,
				Content: []llm.Content{
					llm.TextContent{Text: "Please read the file at /tmp/test.txt"},
				},
			},
		},
		ToolDefinitions: []llm.ToolDefinition{
			{
				Name:        "readfile",
				Description: "Read a file from the filesystem",
				Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			},
		},
	}

	events, err := client.Stream(context.Background(), params)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var gotToolUse bool
	var gotStop bool
	var toolEvent llm.ToolUseEvent

	for evt := range events {
		switch e := evt.(type) {
		case llm.TextDelta:
			fmt.Print(e.Text)
		case llm.ToolUseEvent:
			gotToolUse = true
			toolEvent = e
			t.Logf("[tool_use] id=%s name=%s input=%s", e.ID, e.Name, e.Input)
		case llm.MessageStop:
			gotStop = true
			t.Logf("[stop] reason=%s input_tokens=%d output_tokens=%d",
				e.StopReason, e.Usage.InputTokens, e.Usage.OutputTokens)
		case llm.ErrorEvent:
			t.Fatalf("stream error: %v", e.Err)
		}
	}

	fmt.Println()

	if !gotToolUse {
		t.Error("no ToolUseEvent received")
	}
	if !gotStop {
		t.Error("no MessageStop received")
	}
	if gotToolUse {
		if toolEvent.Name != "readfile" {
			t.Errorf("tool name = %q, want %q", toolEvent.Name, "readfile")
		}
		if toolEvent.ID == "" {
			t.Error("tool ID is empty")
		}
		if len(toolEvent.Input) == 0 {
			t.Error("tool input is empty")
		}
	}
}

func TestStreamMock(t *testing.T) {
	chunks := []string{
		`{"model":"llama3.1","message":{"role":"assistant","content":"Hello"},"done":false}`,
		`{"model":"llama3.1","message":{"role":"assistant","content":" there"},"done":false}`,
		`{"model":"llama3.1","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":10,"eval_count":2}`,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
		}
	}))
	defer server.Close()

	client := ollama.New(nil, server.URL)

	params := llm.RequestParams{
		Model:     "llama3.1",
		MaxTokens: 64,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.Content{llm.TextContent{Text: "hi"}}},
		},
	}

	events, err := client.Stream(context.Background(), params)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var texts []string
	var stopEvent llm.MessageStop
	var gotStop bool

	for evt := range events {
		switch e := evt.(type) {
		case llm.TextDelta:
			texts = append(texts, e.Text)
		case llm.MessageStop:
			gotStop = true
			stopEvent = e
		case llm.ErrorEvent:
			t.Fatalf("stream error: %v", e.Err)
		}
	}

	if got := strings.Join(texts, ""); got != "Hello there" {
		t.Errorf("text = %q, want %q", got, "Hello there")
	}
	if !gotStop {
		t.Fatal("no MessageStop received")
	}
	if stopEvent.StopReason != "end_turn" {
		t.Errorf("stop reason = %q, want %q", stopEvent.StopReason, "end_turn")
	}
	if stopEvent.Usage.InputTokens != 10 {
		t.Errorf("input tokens = %d, want 10", stopEvent.Usage.InputTokens)
	}
	if stopEvent.Usage.OutputTokens != 2 {
		t.Errorf("output tokens = %d, want 2", stopEvent.Usage.OutputTokens)
	}
}

func TestStreamMockToolCall(t *testing.T) {
	chunks := []string{
		`{"model":"llama3.1","message":{"role":"assistant","content":""},"done":false}`,
		`{"model":"llama3.1","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"readfile","arguments":{"path":"/tmp/test.txt"}}}]},"done":true,"prompt_eval_count":15,"eval_count":5}`,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
		}
	}))
	defer server.Close()

	client := ollama.New(nil, server.URL)

	params := llm.RequestParams{
		Model:     "llama3.1",
		MaxTokens: 256,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.Content{llm.TextContent{Text: "read /tmp/test.txt"}}},
		},
		ToolDefinitions: []llm.ToolDefinition{
			{
				Name:        "readfile",
				Description: "Read a file",
				Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			},
		},
	}

	events, err := client.Stream(context.Background(), params)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var gotToolUse bool
	var gotStop bool
	var toolEvent llm.ToolUseEvent

	for evt := range events {
		switch e := evt.(type) {
		case llm.ToolUseEvent:
			gotToolUse = true
			toolEvent = e
		case llm.MessageStop:
			gotStop = true
			if e.StopReason != "tool_use" {
				t.Errorf("stop reason = %q, want %q", e.StopReason, "tool_use")
			}
		case llm.ErrorEvent:
			t.Fatalf("stream error: %v", e.Err)
		}
	}

	if !gotToolUse {
		t.Fatal("no ToolUseEvent received")
	}
	if !gotStop {
		t.Fatal("no MessageStop received")
	}
	if toolEvent.Name != "readfile" {
		t.Errorf("tool name = %q, want %q", toolEvent.Name, "readfile")
	}
	if toolEvent.ID != "call_0" {
		t.Errorf("tool ID = %q, want %q", toolEvent.ID, "call_0")
	}

	var args map[string]string
	if err := json.Unmarshal(toolEvent.Input, &args); err != nil {
		t.Fatalf("unmarshal tool input: %v", err)
	}
	if args["path"] != "/tmp/test.txt" {
		t.Errorf("tool path = %q, want %q", args["path"], "/tmp/test.txt")
	}
}

func TestStreamAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
	}))
	defer server.Close()

	client := ollama.New(nil, server.URL)

	params := llm.RequestParams{
		Model:     "nonexistent-model",
		MaxTokens: 16,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.Content{llm.TextContent{Text: "hi"}}},
		},
	}

	_, err := client.Stream(context.Background(), params)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("error = %v, want to contain 'status 404'", err)
	}
}
