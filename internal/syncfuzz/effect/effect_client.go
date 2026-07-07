package effect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type EffectResource struct {
	ID        string         `json:"id"`
	RequestID string         `json:"requestId,omitempty"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt string         `json:"createdAt"`
}

// AuthorityToken represents C-state in the roadmap: capabilities that may be
// issued by an external authority and consumed independently from agent state.
type AuthorityToken struct {
	Token      string `json:"token"`
	Scope      string `json:"scope"`
	Subject    string `json:"subject"`
	Consumed   bool   `json:"consumed"`
	ConsumedBy string `json:"consumedBy,omitempty"`
	ConsumedAt string `json:"consumedAt,omitempty"`
	IssuedAt   string `json:"issuedAt"`
}

// ExternalState is the current MVP's X/C state projection. It deliberately
// lives behind EffectBackend so testcases can swap memory and HTTP services.
type ExternalState struct {
	Effects struct {
		Resources []EffectResource `json:"resources"`
		Events    []map[string]any `json:"events"`
	} `json:"effects"`
	Authority struct {
		Tokens []AuthorityToken `json:"tokens"`
	} `json:"authority"`
}

type CreateResourceResponse struct {
	Resource         EffectResource `json:"resource"`
	IdempotentReplay bool           `json:"idempotentReplay"`
}

type IssueTokenResponse struct {
	Token AuthorityToken `json:"token"`
}

type ConsumeTokenResponse struct {
	Token    AuthorityToken `json:"token"`
	Error    string         `json:"error,omitempty"`
	Accepted bool           `json:"accepted"`
}

// EffectBackend is the narrow interface used by effect-oriented testcases.
// Real framework tests should only need to replace this boundary, not the
// oracle or trace format.
type EffectBackend interface {
	Reset(context.Context) error
	CreateResource(context.Context, map[string]any) (*CreateResourceResponse, error)
	IssueToken(context.Context, map[string]any) (*IssueTokenResponse, error)
	ConsumeToken(context.Context, map[string]any) (*ConsumeTokenResponse, error)
	State(context.Context) (ExternalState, error)
	Close()
}

type HTTPEffectBackend struct {
	baseURL string
}

func NewHTTPEffectBackend(baseURL string) *HTTPEffectBackend {
	return &HTTPEffectBackend{baseURL: TrimURL(baseURL)}
}

func (b *HTTPEffectBackend) Reset(ctx context.Context) error {
	return resetExternalState(ctx, b.baseURL)
}

func (b *HTTPEffectBackend) CreateResource(ctx context.Context, body map[string]any) (*CreateResourceResponse, error) {
	return createExternalResource(ctx, b.baseURL, body)
}

func (b *HTTPEffectBackend) IssueToken(ctx context.Context, body map[string]any) (*IssueTokenResponse, error) {
	return issueAuthorityToken(ctx, b.baseURL, body)
}

func (b *HTTPEffectBackend) ConsumeToken(ctx context.Context, body map[string]any) (*ConsumeTokenResponse, error) {
	return consumeAuthorityToken(ctx, b.baseURL, body)
}

func (b *HTTPEffectBackend) State(ctx context.Context) (ExternalState, error) {
	return fetchExternalState(ctx, b.baseURL)
}

func (b *HTTPEffectBackend) Close() {}

type MemoryEffectBackend struct {
	state  ExternalState
	nextID int
}

// MemoryEffectBackend keeps the MVP runnable inside restricted sandboxes where
// listening on localhost may be blocked. It still models external state as
// non-rollbackable relative to the simulated agent checkpoint.
func NewMemoryEffectBackend() *MemoryEffectBackend {
	return &MemoryEffectBackend{state: EmptyExternalState(), nextID: 1}
}

func (b *MemoryEffectBackend) Reset(context.Context) error {
	b.state = EmptyExternalState()
	b.nextID = 1
	return nil
}

func (b *MemoryEffectBackend) CreateResource(_ context.Context, body map[string]any) (*CreateResourceResponse, error) {
	requestID, _ := body["requestId"].(string)
	for _, resource := range b.state.Effects.Resources {
		if requestID != "" && resource.RequestID == requestID {
			// Idempotency works only when replay reuses the same request ID.
			// action-replay intentionally changes the request ID to trigger a bug.
			return &CreateResourceResponse{Resource: resource, IdempotentReplay: true}, nil
		}
	}

	kind, _ := body["kind"].(string)
	if kind == "" {
		kind = "generic"
	}
	payload, _ := body["payload"].(map[string]any)
	resource := EffectResource{
		ID:        fmt.Sprintf("res_%d", b.nextID),
		RequestID: requestID,
		Kind:      kind,
		Payload:   payload,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	b.nextID++
	b.state.Effects.Resources = append(b.state.Effects.Resources, resource)
	return &CreateResourceResponse{Resource: resource, IdempotentReplay: false}, nil
}

func (b *MemoryEffectBackend) IssueToken(_ context.Context, body map[string]any) (*IssueTokenResponse, error) {
	scope, _ := body["scope"].(string)
	if scope == "" {
		scope = "default"
	}
	subject, _ := body["subject"].(string)
	if subject == "" {
		subject = "agent"
	}
	token := AuthorityToken{
		Token:    fmt.Sprintf("tok_%d", len(b.state.Authority.Tokens)+1),
		Scope:    scope,
		Subject:  subject,
		Consumed: false,
		IssuedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	b.state.Authority.Tokens = append(b.state.Authority.Tokens, token)
	return &IssueTokenResponse{Token: token}, nil
}

func (b *MemoryEffectBackend) ConsumeToken(_ context.Context, body map[string]any) (*ConsumeTokenResponse, error) {
	tokenValue, _ := body["token"].(string)
	operation, _ := body["operation"].(string)
	if operation == "" {
		operation = "unknown"
	}

	for i := range b.state.Authority.Tokens {
		token := &b.state.Authority.Tokens[i]
		if token.Token != tokenValue {
			continue
		}
		if token.Consumed {
			// This rejection is expected in the known-answer case. It proves the
			// authority server remembers a consume that agent replay forgot.
			return &ConsumeTokenResponse{
				Token:    *token,
				Error:    "token_already_consumed",
				Accepted: false,
			}, nil
		}
		token.Consumed = true
		token.ConsumedBy = operation
		token.ConsumedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return &ConsumeTokenResponse{Token: *token, Accepted: true}, nil
	}

	return &ConsumeTokenResponse{Error: "token_not_found", Accepted: false}, nil
}

func (b *MemoryEffectBackend) State(context.Context) (ExternalState, error) {
	return b.state, nil
}

func (b *MemoryEffectBackend) Close() {}

func resetExternalState(ctx context.Context, baseURL string) error {
	var out map[string]any
	return postJSON(ctx, baseURL+"/reset", map[string]any{}, &out)
}

func createExternalResource(ctx context.Context, baseURL string, body map[string]any) (*CreateResourceResponse, error) {
	var out CreateResourceResponse
	if err := postJSON(ctx, baseURL+"/effect/resources", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func issueAuthorityToken(ctx context.Context, baseURL string, body map[string]any) (*IssueTokenResponse, error) {
	var out IssueTokenResponse
	if err := postJSON(ctx, baseURL+"/authority/tokens", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func consumeAuthorityToken(ctx context.Context, baseURL string, body map[string]any) (*ConsumeTokenResponse, error) {
	var out ConsumeTokenResponse
	status, err := postJSONStatus(ctx, baseURL+"/authority/consume", body, &out)
	if err != nil {
		return nil, err
	}
	out.Accepted = status >= 200 && status < 300
	if out.Error == "" && !out.Accepted {
		out.Error = "consume_rejected"
	}
	return &out, nil
}

func fetchExternalState(ctx context.Context, baseURL string) (ExternalState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/state", nil)
	if err != nil {
		return ExternalState{}, err
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ExternalState{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExternalState{}, fmt.Errorf("GET /state returned %s", resp.Status)
	}

	var state ExternalState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return ExternalState{}, err
	}
	return state, nil
}

func postJSON(ctx context.Context, url string, body any, out any) error {
	status, err := postJSONStatus(ctx, url, body, out)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("POST %s returned HTTP %d", url, status)
	}
	return nil
}

func postJSONStatus(ctx context.Context, url string, body any, out any) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("content-type", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&problem); err == nil {
			rawProblem, _ := json.Marshal(problem)
			if out != nil {
				// Some negative responses are semantically meaningful, such as
				// token_already_consumed. Decode them instead of hiding them as
				// transport-only errors.
				_ = json.Unmarshal(rawProblem, out)
			}
		}
		return resp.StatusCode, nil
	}

	if out == nil {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, nil
}

func TrimURL(value string) string {
	return strings.TrimRight(value, "/")
}

func EmptyExternalState() ExternalState {
	var state ExternalState
	state.Effects.Resources = []EffectResource{}
	state.Effects.Events = []map[string]any{}
	state.Authority.Tokens = []AuthorityToken{}
	return state
}
