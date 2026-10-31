package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	toolPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	schemaName   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
)

// Client performs one bounded Meta Responses request at a time.
type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) (*Client, error) {
	n, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	// Test endpoints may use synthetic model names. Public provider traffic is
	// locked to the operator-selected contributor model; no model fallback.
	if u, err := url.Parse(n.BaseURL); err != nil || (!isLoopback(u.Hostname()) && n.Model != DefaultModel) {
		return nil, errors.New("only muse-spark-1.3-contributor is enabled")
	}
	client := n.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: n.RequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Client{cfg: n, http: client}, nil
}

func (c *Client) Model() string { return c.cfg.Model }

type wireError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param"`
	Code    string `json:"code"`
}

func (c *Client) Complete(ctx context.Context, req Request) (Response, error) {
	if ctx.Err() != nil {
		return Response{}, ctx.Err()
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.cfg.Model
	}
	if !modelPattern.MatchString(model) || model != c.cfg.Model {
		return Response{}, errors.New("invalid model identifier")
	}
	if err := validateRequest(req); err != nil {
		return Response{}, err
	}
	body, err := encodeResponses(req, model)
	if err != nil {
		return Response{}, errors.New("cannot encode model request")
	}
	if len(body) > c.cfg.MaxRequestBytes {
		return Response{}, &TransportError{Code: "request_too_large", Message: "model request exceeds size limit"}
	}
	requestHash := hash(body)

	callCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.cfg.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return Response{}, errors.New("cannot build model request")
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "plenilune-meta/0.1")

	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Response{}, ctxErr
		}
		var urlErr interface{ Timeout() bool }
		if errors.As(err, &urlErr) && urlErr.Timeout() {
			return Response{}, &TransportError{Code: "timeout", Message: "model request timed out", RequestSHA256: requestHash}
		}
		// Policy allows one retry only for transient HTTP statuses, not for
		// transport failures.
		return Response{}, &TransportError{Code: "network", Message: "cannot reach Meta Model API", RequestSHA256: requestHash}
	}
	data, readErr := io.ReadAll(io.LimitReader(httpResp.Body, c.cfg.MaxResponseBytes+1))
	// A close error after a bounded read is not actionable; the body is not reused.
	_ = httpResp.Body.Close()
	if readErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Response{}, ctxErr
		}
		return Response{}, &TransportError{Code: "read_failed", Message: "cannot read Meta API response", RequestSHA256: requestHash}
	}
	if int64(len(data)) > c.cfg.MaxResponseBytes {
		return Response{}, &TransportError{Code: "response_too_large", Message: "Meta API response exceeds size limit", RequestSHA256: requestHash}
	}
	responseHash := hash(data)
	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		return Response{}, c.statusError(httpResp.StatusCode, data, requestHash, responseHash)
	}
	resp, err := decodeResponses(data, model)
	if err != nil {
		var envelope struct {
			Error *wireError `json:"error"`
		}
		if json.Unmarshal(data, &envelope) == nil && envelope.Error != nil {
			return Response{}, c.envelopeError(httpResp.StatusCode, envelope.Error, requestHash, responseHash)
		}
		var te *TransportError
		if errors.As(err, &te) {
			te.RequestSHA256 = requestHash
		}
		return Response{}, err
	}
	resp.RequestSHA256, resp.ResponseSHA256 = requestHash, responseHash
	return resp, nil
}

func validateRequest(req Request) error {
	if len(req.Messages) == 0 || len(req.Messages) > 64 {
		return errors.New("model request requires 1 to 64 messages")
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "system", "user", "assistant":
		case "tool":
			if m.ToolCallID == "" {
				return errors.New("tool message requires tool_call_id")
			}
		default:
			return errors.New("unsupported message role")
		}
	}
	if len(req.Tools) > 8 {
		return errors.New("too many tool definitions")
	}
	for _, t := range req.Tools {
		if t.Type != "function" || !toolPattern.MatchString(t.Function.Name) {
			return errors.New("invalid tool definition")
		}
		if len(t.Function.Description) > 4096 {
			return errors.New("tool description is too long")
		}
		var obj map[string]json.RawMessage
		if len(t.Function.Parameters) == 0 || json.Unmarshal(t.Function.Parameters, &obj) != nil || obj == nil {
			return errors.New("tool parameters must be a JSON object")
		}
	}
	if req.ResponseFormat != nil {
		if req.ResponseFormat.Type != "json_schema" || req.ResponseFormat.JSONSchema == nil {
			return errors.New("unsupported response format")
		}
		if !schemaName.MatchString(req.ResponseFormat.JSONSchema.Name) {
			return errors.New("invalid response schema name")
		}
		var obj map[string]json.RawMessage
		if len(req.ResponseFormat.JSONSchema.Schema) == 0 ||
			json.Unmarshal(req.ResponseFormat.JSONSchema.Schema, &obj) != nil || obj == nil {
			return errors.New("response format schema must be a JSON object")
		}
	}
	if req.MaxTokens < 1 || req.MaxTokens > 4096 {
		return errors.New("max_tokens is required and must be between 1 and 4096")
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		return errors.New("temperature is outside the supported range")
	}
	return nil
}

func toolChoice(tools []Tool) string {
	if len(tools) == 0 {
		return ""
	}
	return "auto"
}

func (c *Client) statusError(status int, data []byte, requestHash, responseHash string) error {
	var envelope struct {
		Error *wireError `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil && envelope.Error != nil {
		return c.envelopeError(status, envelope.Error, requestHash, responseHash)
	}
	return &StatusError{
		Status:         status,
		Code:           fmt.Sprintf("http_%d", status),
		Message:        sanitize(http.StatusText(status), c.cfg.APIKey, 200),
		Retriable:      status == 429 || status >= 500,
		RequestSHA256:  requestHash,
		ResponseSHA256: responseHash,
	}
}

func (c *Client) envelopeError(status int, wire *wireError, requestHash, responseHash string) error {
	code := sanitize(wire.Code, c.cfg.APIKey, 64)
	if code == "" {
		code = fmt.Sprintf("http_%d", status)
	}
	if len(code) > 64 {
		code = code[:64]
	}
	return &StatusError{
		Status:         status,
		Code:           code,
		Type:           sanitize(wire.Type, c.cfg.APIKey, 64),
		Message:        sanitize(wire.Message, c.cfg.APIKey, 240),
		Retriable:      status == 429 || status >= 500,
		RequestSHA256:  requestHash,
		ResponseSHA256: responseHash,
	}
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
