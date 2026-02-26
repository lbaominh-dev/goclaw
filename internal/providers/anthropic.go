package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultClaudeModel   = "claude-sonnet-4-5-20250929"
	anthropicAPIBase     = "https://api.anthropic.com/v1"
	anthropicAPIVersion  = "2023-06-01"
)

// AnthropicProvider implements Provider using the Anthropic Claude API via net/http.
type AnthropicProvider struct {
	apiKey       string
	defaultModel string
	client       *http.Client
	retryConfig  RetryConfig
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey string, opts ...AnthropicOption) *AnthropicProvider {
	p := &AnthropicProvider{
		apiKey:       apiKey,
		defaultModel: defaultClaudeModel,
		client:       &http.Client{Timeout: 120 * time.Second},
		retryConfig:  DefaultRetryConfig(),
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

type AnthropicOption func(*AnthropicProvider)

func WithAnthropicModel(model string) AnthropicOption {
	return func(p *AnthropicProvider) { p.defaultModel = model }
}

func (p *AnthropicProvider) Name() string        { return "anthropic" }
func (p *AnthropicProvider) DefaultModel() string { return p.defaultModel }

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	body := p.buildRequestBody(model, req, false)

	return RetryDo(ctx, p.retryConfig, func() (*ChatResponse, error) {
		respBody, err := p.doRequest(ctx, body)
		if err != nil {
			return nil, err
		}
		defer respBody.Close()

		var resp anthropicResponse
		if err := json.NewDecoder(respBody).Decode(&resp); err != nil {
			return nil, fmt.Errorf("anthropic: decode response: %w", err)
		}

		return p.parseResponse(&resp), nil
	})
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}

	body := p.buildRequestBody(model, req, true)

	// Retry only the connection phase; once streaming starts, no retry.
	respBody, err := RetryDo(ctx, p.retryConfig, func() (io.ReadCloser, error) {
		return p.doRequest(ctx, body)
	})
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	result := &ChatResponse{FinishReason: "stop"}
	// Accumulate raw JSON fragments for each tool call by index
	toolCallJSON := make(map[int]string)

	scanner := bufio.NewScanner(respBody)
	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()

		// Track event type
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		switch currentEvent {
		case "message_start":
			var ev anthropicMessageStartEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if result.Usage == nil {
					result.Usage = &Usage{}
				}
				if ev.Message.Usage.InputTokens > 0 {
					result.Usage.PromptTokens = ev.Message.Usage.InputTokens
				}
				result.Usage.CacheCreationTokens = ev.Message.Usage.CacheCreationInputTokens
				result.Usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
			}

		case "content_block_start":
			var ev anthropicContentBlockStartEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if ev.ContentBlock.Type == "tool_use" {
					result.ToolCalls = append(result.ToolCalls, ToolCall{
						ID:        ev.ContentBlock.ID,
						Name:      ev.ContentBlock.Name,
						Arguments: make(map[string]interface{}),
					})
				}
			}

		case "content_block_delta":
			var ev anthropicContentBlockDeltaEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if ev.Delta.Type == "text_delta" {
					result.Content += ev.Delta.Text
					if onChunk != nil {
						onChunk(StreamChunk{Content: ev.Delta.Text})
					}
				} else if ev.Delta.Type == "input_json_delta" {
					if len(result.ToolCalls) > 0 {
						idx := len(result.ToolCalls) - 1
						toolCallJSON[idx] += ev.Delta.PartialJSON
					}
				}
			}

		case "message_delta":
			var ev anthropicMessageDeltaEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if ev.Delta.StopReason != "" {
					switch ev.Delta.StopReason {
					case "tool_use":
						result.FinishReason = "tool_calls"
					case "max_tokens":
						result.FinishReason = "length"
					default:
						result.FinishReason = "stop"
					}
				}
				if ev.Usage.OutputTokens > 0 {
					if result.Usage == nil {
						result.Usage = &Usage{}
					}
					result.Usage.CompletionTokens = ev.Usage.OutputTokens
				}
			}

		case "error":
			var ev anthropicErrorEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				return nil, fmt.Errorf("anthropic stream error: %s: %s", ev.Error.Type, ev.Error.Message)
			}

		case "message_stop":
			// Stream complete
		}
	}

	// Parse accumulated tool call JSON arguments
	for i, rawJSON := range toolCallJSON {
		if rawJSON != "" {
			args := make(map[string]interface{})
			_ = json.Unmarshal([]byte(rawJSON), &args)
			result.ToolCalls[i].Arguments = args
		}
	}

	if result.Usage != nil {
		result.Usage.TotalTokens = result.Usage.PromptTokens + result.Usage.CompletionTokens
	}

	if onChunk != nil {
		onChunk(StreamChunk{Done: true})
	}

	return result, nil
}

func (p *AnthropicProvider) buildRequestBody(model string, req ChatRequest, stream bool) map[string]interface{} {
	// Separate system messages and build conversation messages
	var systemBlocks []map[string]interface{}
	var messages []map[string]interface{}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			systemBlocks = append(systemBlocks, map[string]interface{}{
				"type": "text",
				"text": msg.Content,
			})

		case "user":
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": msg.Content,
			})

		case "assistant":
			var blocks []map[string]interface{}
			if msg.Content != "" {
				blocks = append(blocks, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Arguments,
				})
			}
			messages = append(messages, map[string]interface{}{
				"role":    "assistant",
				"content": blocks,
			})

		case "tool":
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": msg.ToolCallID,
						"content":     msg.Content,
					},
				},
			})
		}
	}

	body := map[string]interface{}{
		"model":         model,
		"max_tokens":    4096,
		"messages":      messages,
		"cache_control": map[string]interface{}{"type": "ephemeral"},
	}

	if stream {
		body["stream"] = true
	}

	if len(systemBlocks) > 0 {
		body["system"] = systemBlocks
	}

	// Translate tools to Anthropic format
	if len(req.Tools) > 0 {
		var tools []map[string]interface{}
		for _, t := range req.Tools {
			cleanedParams := CleanSchemaForProvider("anthropic", t.Function.Parameters)
			tool := map[string]interface{}{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": cleanedParams,
			}
			tools = append(tools, tool)
		}
		body["tools"] = tools
	}

	// Merge options
	if v, ok := req.Options["max_tokens"]; ok {
		body["max_tokens"] = v
	}
	if v, ok := req.Options["temperature"]; ok {
		body["temperature"] = v
	}

	return body
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body interface{}) (io.ReadCloser, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIBase+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		retryAfter := ParseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &HTTPError{
			Status:     resp.StatusCode,
			Body:       fmt.Sprintf("anthropic: %s", string(respBody)),
			RetryAfter: retryAfter,
		}
	}

	return resp.Body, nil
}

func (p *AnthropicProvider) parseResponse(resp *anthropicResponse) *ChatResponse {
	result := &ChatResponse{}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			args := make(map[string]interface{})
			_ = json.Unmarshal(block.Input, &args)
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	switch resp.StopReason {
	case "tool_use":
		result.FinishReason = "tool_calls"
	case "max_tokens":
		result.FinishReason = "length"
	default:
		result.FinishReason = "stop"
	}

	result.Usage = &Usage{
		PromptTokens:        resp.Usage.InputTokens,
		CompletionTokens:    resp.Usage.OutputTokens,
		TotalTokens:         resp.Usage.InputTokens + resp.Usage.OutputTokens,
		CacheCreationTokens: resp.Usage.CacheCreationInputTokens,
		CacheReadTokens:     resp.Usage.CacheReadInputTokens,
	}

	return result
}

// --- Anthropic API types (internal) ---

type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                 `json:"stop_reason"`
	Usage      anthropicUsage         `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// --- Streaming event types ---

type anthropicMessageStartEvent struct {
	Message struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
}

type anthropicContentBlockStartEvent struct {
	Index        int                   `json:"index"`
	ContentBlock anthropicContentBlock `json:"content_block"`
}

type anthropicContentBlockDeltaEvent struct {
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthropicMessageDeltaEvent struct {
	Delta struct {
		StopReason string `json:"stop_reason,omitempty"`
	} `json:"delta"`
	Usage anthropicUsage `json:"usage"`
}

type anthropicErrorEvent struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
