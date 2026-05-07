package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/psuijk/golem/internal/llm"
)

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaToolFunc `json:"function"`
}

type ollamaToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaStreamChunk struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func New(client *http.Client, baseURL string) *Client {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Client{httpClient: client, baseURL: baseURL}
}

func buildMessages(req llm.RequestParams) []ollamaMessage {
	var msgs []ollamaMessage

	if req.SystemPrompt != "" {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: req.SystemPrompt})
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleUser:
			for _, c := range msg.Content {
				switch content := c.(type) {
				case llm.TextContent:
					msgs = append(msgs, ollamaMessage{Role: "user", Content: content.Text})
				case llm.ToolResultContent:
					msgs = append(msgs, ollamaMessage{Role: "tool", Content: content.Content})
				}
			}
		case llm.RoleAssistant:
			var text string
			var toolCalls []ollamaToolCall
			for _, c := range msg.Content {
				switch content := c.(type) {
				case llm.TextContent:
					text = content.Text
				case llm.ToolUseContent:
					toolCalls = append(toolCalls, ollamaToolCall{
						Function: ollamaFunction{
							Name:      content.Name,
							Arguments: content.Input,
						},
					})
				}
			}
			m := ollamaMessage{Role: "assistant", Content: text}
			if len(toolCalls) > 0 {
				m.ToolCalls = toolCalls
			}
			msgs = append(msgs, m)
		}
	}

	return msgs
}

func buildRequest(req llm.RequestParams) ollamaRequest {
	var tools []ollamaTool
	for _, td := range req.ToolDefinitions {
		tools = append(tools, ollamaTool{
			Type: "function",
			Function: ollamaToolFunc{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  td.Schema,
			},
		})
	}

	r := ollamaRequest{
		Model:    req.Model,
		Messages: buildMessages(req),
		Stream:   true,
		Tools:    tools,
	}

	if req.Temperature != 0 || req.MaxTokens != 0 {
		r.Options = &ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		}
	}

	return r
}

func (c *Client) Stream(ctx context.Context, request llm.RequestParams) (<-chan llm.StreamEvent, error) {
	req := buildRequest(request)
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: API error (status %d): %s", resp.StatusCode, body)
	}

	out := make(chan llm.StreamEvent)

	go func() {
		defer resp.Body.Close()
		defer close(out)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var chunk ollamaStreamChunk
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				out <- llm.ErrorEvent{Err: fmt.Errorf("ollama: parse response: %w", err)}
				return
			}

			if chunk.Message.Content != "" {
				out <- llm.TextDelta{Text: chunk.Message.Content}
			}

			if chunk.Done {
				for i, tc := range chunk.Message.ToolCalls {
					out <- llm.ToolUseEvent{
						ID:    fmt.Sprintf("call_%d", i),
						Name:  tc.Function.Name,
						Input: tc.Function.Arguments,
					}
				}

				stopReason := "end_turn"
				if len(chunk.Message.ToolCalls) > 0 {
					stopReason = "tool_use"
				}

				out <- llm.MessageStop{
					StopReason: stopReason,
					Usage: llm.Usage{
						InputTokens:  chunk.PromptEvalCount,
						OutputTokens: chunk.EvalCount,
					},
				}
			}
		}
	}()

	return out, nil
}

var _ llm.Provider = (*Client)(nil)
