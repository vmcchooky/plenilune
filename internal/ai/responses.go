package ai

import (
	"encoding/json"
	"errors"
	"strings"
)

// This adapter uses stateless Responses: encrypted reasoning stays opaque and
// is replayed inside a bounded session, never persisted in evidence or logs.
func encodeResponses(req Request, model string) ([]byte, error) {
	input := make([]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "assistant" && len(m.ReplayItems) > 0 {
			for _, raw := range m.ReplayItems {
				if !json.Valid(raw) {
					return nil, errors.New("invalid replay item")
				}
				input = append(input, raw)
			}
			continue
		}
		if m.Role == "tool" {
			input = append(input, map[string]any{"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content})
			continue
		}
		if m.Content != "" {
			kind := "input_text"
			if m.Role == "assistant" {
				kind = "output_text"
			}
			input = append(input, map[string]any{"role": m.Role, "content": []any{map[string]string{"type": kind, "text": m.Content}}})
		}
		for _, call := range m.ToolCalls {
			input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments})
		}
	}
	max := req.MaxTokens
	if max == 0 {
		max = 1200
	}
	body := map[string]any{
		"model": model, "input": input, "stream": false, "store": false,
		"include":   []string{"reasoning.encrypted_content"},
		"reasoning": map[string]string{"effort": "medium"}, "max_output_tokens": max,
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if len(req.Tools) > 0 {
		tools := make([]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "name": t.Function.Name, "description": t.Function.Description, "parameters": t.Function.Parameters})
		}
		body["tools"], body["tool_choice"] = tools, "auto"
	}
	if f := req.ResponseFormat; f != nil {
		body["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": f.JSONSchema.Name, "schema": f.JSONSchema.Schema}}
	}
	return json.Marshal(body)
}

func decodeResponses(data []byte, model string) (Response, error) {
	bad := func(code, message string) (Response, error) {
		return Response{}, &TransportError{Code: code, Message: message}
	}
	var wire struct {
		ID     string            `json:"id"`
		Model  string            `json:"model"`
		Status string            `json:"status"`
		Output []json.RawMessage `json:"output"`
		Usage  struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
		Incomplete struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	}
	if json.Unmarshal(data, &wire) != nil {
		return bad("invalid_response_json", "Meta API response is not valid JSON")
	}
	if wire.Model != "" && wire.Model != model {
		return bad("invalid_response", "Meta API returned a different model")
	}
	if wire.Status != "completed" && wire.Status != "incomplete" {
		return bad("invalid_response", "Meta API response did not complete")
	}
	if wire.ID == "" || len(wire.Output) == 0 {
		return bad("invalid_response", "Meta API response has no completed output")
	}
	r := Response{ID: wire.ID, Model: model, FinishReason: "stop", Usage: Usage{PromptTokens: wire.Usage.Input, CompletionTokens: wire.Usage.Output, TotalTokens: wire.Usage.Total}}
	if r.Usage.PromptTokens < 0 || r.Usage.CompletionTokens < 0 || r.Usage.TotalTokens < 0 {
		return bad("invalid_response", "Meta API returned invalid usage")
	}
	if r.Usage.TotalTokens == 0 {
		r.Usage.TotalTokens = r.Usage.PromptTokens + r.Usage.CompletionTokens
	}
	seen := map[string]bool{}
	var text strings.Builder
	for _, raw := range wire.Output {
		var item struct {
			Type      string `json:"type"`
			Role      string `json:"role"`
			Status    string `json:"status"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Encrypted string `json:"encrypted_content"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(raw, &item) != nil {
			return bad("invalid_response", "Meta API returned an invalid output item")
		}
		switch item.Type {
		case "reasoning":
			if item.Encrypted == "" {
				continue
			}
			// The protocol requires summary even when no summary was requested.
			var replay map[string]json.RawMessage
			_ = json.Unmarshal(raw, &replay)
			if len(replay["summary"]) == 0 || string(replay["summary"]) == "null" {
				replay["summary"] = json.RawMessage(`[]`)
			}
			normalized, err := json.Marshal(replay)
			if err != nil {
				return bad("invalid_response", "invalid reasoning envelope")
			}
			r.ReplayItems = append(r.ReplayItems, normalized)
			continue
		case "message":
			if item.Role != "assistant" || (item.Status != "completed" && wire.Status != "incomplete") {
				return bad("invalid_response", "Meta API returned an incomplete message")
			}
			for _, block := range item.Content {
				switch block.Type {
				case "output_text":
					text.WriteString(block.Text)
				case "refusal":
					r.FinishReason = "content_filter"
				default:
					return bad("invalid_response", "unsupported model content")
				}
			}
		case "function_call":
			if item.Status != "completed" || wire.Status != "completed" || item.CallID == "" || len(item.CallID) > 128 || seen[item.CallID] || !toolPattern.MatchString(item.Name) || len(item.Arguments) > MaxToolArgumentBytes || !json.Valid([]byte(item.Arguments)) {
				return bad("invalid_response", "Meta API returned an invalid tool call")
			}
			seen[item.CallID] = true
			r.ToolCalls = append(r.ToolCalls, ToolCall{ID: item.CallID, Type: "function", Function: FunctionCall{Name: item.Name, Arguments: item.Arguments}})
			if len(r.ToolCalls) > MaxToolCalls {
				return bad("invalid_response", "model returned too many tool calls")
			}
		default:
			return bad("invalid_response", "unsupported model output")
		}
		r.ReplayItems = append(r.ReplayItems, append(json.RawMessage(nil), raw...))
	}
	r.Content = text.String()
	if len(r.Content) > MaxContentBytes {
		return bad("content_too_large", "model content exceeds size limit")
	}
	if wire.Status == "incomplete" {
		if wire.Incomplete.Reason != "max_output_tokens" {
			return bad("invalid_response", "Meta API returned incomplete output")
		}
		r.FinishReason = "length"
	} else if len(r.ToolCalls) > 0 && r.FinishReason != "content_filter" {
		r.FinishReason = "tool_calls"
	}
	if r.Content == "" && len(r.ToolCalls) == 0 && r.FinishReason == "stop" {
		return bad("invalid_response", "Meta API returned no usable output")
	}
	return r, nil
}
