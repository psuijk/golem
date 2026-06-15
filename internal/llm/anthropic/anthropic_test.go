package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/psuijk/golem/internal/llm"
	"github.com/psuijk/golem/internal/llm/anthropic"
)

func floatPtr(f float32) *float32 { return &f }

func TestStreamLive(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping live test")
	}

	client, err := anthropic.New(nil, apiKey, "")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	params := llm.RequestParams{
		Model:        "claude-haiku-4-5-20251001",
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
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping live test")
	}

	client, err := anthropic.New(nil, apiKey, "")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	params := llm.RequestParams{
		Model:        "claude-haiku-4-5-20251001",
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

func TestStreamInvalidKey(t *testing.T) {
	client, err := anthropic.New(nil, "sk-ant-not-a-real-key", "")
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	params := llm.RequestParams{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 16,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.Content{llm.TextContent{Text: "hi"}}},
		},
	}

	_, err = client.Stream(context.Background(), params)
	if err == nil {
		t.Fatal("expected error for invalid API key, got nil")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("error = %v, want to contain 'status 401'", err)
	}
}
