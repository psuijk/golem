package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/psuijk/golem/internal/llm"
)

type sseData struct {
	Type         string           `json:"type"`
	Delta        *sseDelta        `json:"delta,omitempty"`
	ContentBlock *sseContentBlock `json:"content_block,omitempty"`
	Usage        *llm.Usage       `json:"usage,omitempty"`
}

type sseDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type sseContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	Stream      bool               `json:"stream"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float32            `json:"temperature"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
}

type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

func New(client *http.Client, apiKey string, baseURL string) (*Client, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	if apiKey == "" {
		return nil, errors.New("anthropic: api key required")
	}

	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Client{httpClient: client, apiKey: apiKey, baseURL: baseURL}, nil
}

func buildRequest(req llm.RequestParams) anthropicRequest {
	var msgs []anthropicMessage
	for _, msg := range req.Messages {
		var parts []anthropicContent
		for _, c := range msg.Content {
			switch content := c.(type) {
			case llm.TextContent:
				parts = append(parts, anthropicContent{Type: "text", Text: content.Text})
			case llm.ToolUseContent:
				parts = append(parts, anthropicContent{Type: "tool_use", ID: content.ID, Name: content.Name, Input: content.Input})
			case llm.ToolResultContent:
				parts = append(parts, anthropicContent{Type: "tool_result", ToolUseID: content.ToolCallID, Content: content.Content, IsError: content.IsError})
			}
		}
		msgs = append(msgs, anthropicMessage{Role: string(msg.Role), Content: parts})
	}

	var toolDefs []anthropicTool
	for _, tool := range req.ToolDefinitions {
		toolDefs = append(toolDefs, anthropicTool{Name: tool.Name, Description: tool.Description, InputSchema: tool.Schema})
	}

	return anthropicRequest{
		Messages:    msgs,
		Model:       req.Model,
		System:      req.SystemPrompt,
		Stream:      true,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Tools:       toolDefs}
}

func (c *Client) Stream(ctx context.Context, request llm.RequestParams) (<-chan llm.StreamEvent, error) {
	req := buildRequest(request)
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("anthropic: reading error response: %w", err)
		}
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic: API error (status %d): %s", resp.StatusCode, body)
	}

	out := make(chan llm.StreamEvent)

	go func() {
		defer resp.Body.Close()
		defer close(out)
		scanner := bufio.NewScanner(resp.Body)
		var eventType string
		var payload string
		var toolID string
		var toolName string
		var toolInput strings.Builder
		for scanner.Scan() {
			line := scanner.Text()

			if after, ok := strings.CutPrefix(line, "event: "); ok {
				eventType = after
				continue
			}

			if after, ok := strings.CutPrefix(line, "data: "); ok {
				payload = after
				var jsonData sseData
				err := json.Unmarshal([]byte(payload), &jsonData)
				if err != nil {
					out <- llm.ErrorEvent{Err: fmt.Errorf("anthropic: parse SSE data: %w", err)}
					return
				}
				switch eventType {
				case "content_block_start":
					if jsonData.ContentBlock.Type == "tool_use" {
						toolID = jsonData.ContentBlock.ID
						toolName = jsonData.ContentBlock.Name
						toolInput.Reset()
					}
				case "content_block_delta":
					if jsonData.Delta.Type == "text_delta" {
						out <- llm.TextDelta{Text: jsonData.Delta.Text}
					} else {
						toolInput.WriteString(jsonData.Delta.PartialJSON)
					}
				case "content_block_stop":
					if toolID != "" {
						out <- llm.ToolUseEvent{ID: toolID, Name: toolName, Input: json.RawMessage(toolInput.String())}
						toolID = ""
					}
				case "message_delta":
					var usage llm.Usage
					if jsonData.Usage != nil {
						usage = *jsonData.Usage
					}
					out <- llm.MessageStop{StopReason: jsonData.Delta.StopReason, Usage: usage}
				}
			}
		}

	}()

	return out, nil
}

var _ llm.Provider = (*Client)(nil)
