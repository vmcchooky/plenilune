package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Completer is the small provider port used by orchestration. Implementations
// perform one model request; retry budgets belong to the caller.
type Completer interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// Request is one Responses request. Content is plain text; domain JSON
// is carried as the caller's content and decoded by the caller.
type Request struct {
	Model          string
	Messages       []Message
	Tools          []Tool
	ResponseFormat *ResponseFormat
	MaxTokens      int
	Temperature    *float64
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// ReplayItems are opaque provider output, retained only inside this session.
	// They preserve encrypted reasoning and function-call pairing; never exported.
	ReplayItems []json.RawMessage `json:"-"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool uses the nested function shape documented for Meta Model API tools.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Response struct {
	ID             string
	Model          string
	Content        string
	ToolCalls      []ToolCall
	FinishReason   string
	Usage          Usage
	RequestSHA256  string
	ResponseSHA256 string
	ReplayItems    []json.RawMessage `json:"-"`
}

// StatusError is a typed, sanitized provider failure. Message never contains
// the API key or an unbounded response body.
type StatusError struct {
	Status         int
	Code           string
	Type           string
	Message        string
	Retriable      bool
	RequestSHA256  string
	ResponseSHA256 string
}

func (e *StatusError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("meta api error (status %d, code %s)", e.Status, e.Code)
	}
	return fmt.Sprintf("meta api error (status %d, code %s): %s", e.Status, e.Code, e.Message)
}

// TransportError covers network and framing failures before a provider status.
type TransportError struct {
	Code          string
	Message       string
	Retriable     bool
	RequestSHA256 string
}

func (e *TransportError) Error() string {
	if e.Message == "" {
		return "meta api transport error: " + e.Code
	}
	return "meta api transport error (" + e.Code + "): " + e.Message
}

// Retriable reports whether policy allows a single retry. Only transient HTTP
// 429/5xx qualify; auth, schema, timeout and malformed responses never retry.
func Retriable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Retriable
	}
	var te *TransportError
	if errors.As(err, &te) {
		return te.Retriable
	}
	return false
}

func sanitize(message, secret string, limit int) string {
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, message)
	if secret != "" {
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
	}
	message = strings.TrimSpace(message)
	if runes := []rune(message); len(runes) > limit {
		message = string(runes[:limit]) + "..."
	}
	return message
}
