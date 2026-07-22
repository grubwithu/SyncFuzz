package target

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunTargetOpenAIContractProposalWritesValidatedCandidates(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	const quote = "A replay from this checkpoint must not retain later PATH mutations."
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("create source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "replay.md"), []byte("# Replay\n"+quote+"\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	client := &http.Client{Transport: contractProposalRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://provider.example/v1/chat/completions" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return contractProposalHTTPResponse(r, http.StatusNotFound, `{"error":"unexpected request"}`), nil
		}
		defer r.Body.Close()
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected authorization header %q", got)
		}
		var payload targetContractProposalOpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
			return contractProposalHTTPResponse(r, http.StatusBadRequest, `{"error":"bad payload"}`), nil
		}
		if payload.Model != "test-model" || payload.Temperature != 0 || payload.ResponseFormat.Type != "json_object" || len(payload.Messages) != 2 {
			t.Errorf("unexpected payload metadata: %#v", payload)
		}
		if !strings.Contains(payload.Messages[0].Content, `"start_line"`) || !strings.Contains(payload.Messages[0].Content, `"source_type"`) || !strings.Contains(payload.Messages[0].Content, "Do not use a field named proposal") {
			t.Errorf("system prompt does not describe the strict candidate schema: %q", payload.Messages[0].Content)
		}
		var input struct {
			ProposalRequest TargetContractProposalRequest `json:"proposal_request"`
		}
		if err := json.Unmarshal([]byte(payload.Messages[1].Content), &input); err != nil {
			t.Errorf("decode proposal request message: %v", err)
		}
		if input.ProposalRequest.TargetID != "langgraph-shell-react" || len(input.ProposalRequest.Sources) != 1 || input.ProposalRequest.Sources[0].SourcePath != "replay.md" {
			t.Errorf("unexpected bounded proposal request: %#v", input.ProposalRequest)
		}

		candidates := TargetContractCandidateSet{
			SchemaVersion: TargetContractCandidateSetSchemaVersion,
			Candidates: []TargetContractCandidate{{
				CandidateID:   "shell-path-replay-reset",
				TargetID:      "langgraph-shell-react",
				TaskID:        PersistentShellReplayTargetTaskID,
				StateSurface:  "shell-session.path",
				LifecycleEdge: "checkpoint->replay",
				Expectation:   TargetContractExpectationReset,
				SourceType:    TargetContractCandidateSourceScenarioAssumption,
				Source: TargetContractCandidateSource{
					SourcePath: "replay.md",
					StartLine:  2,
					EndLine:    2,
					Quote:      quote,
				},
			}},
		}
		rawCandidates, err := json.Marshal(candidates)
		if err != nil {
			t.Fatalf("encode response candidates: %v", err)
		}
		return contractProposalHTTPResponse(r, http.StatusOK, fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, string(rawCandidates))), nil
	})}

	result, err := RunTargetOpenAIContractProposal(context.Background(), TargetOpenAIContractProposalOptions{
		TargetContractProposalOptions: TargetContractProposalOptions{
			RunID:      "openai-proposal",
			TargetID:   "langgraph-shell-react",
			TaskIDs:    []string{PersistentShellReplayTargetTaskID},
			SourceRoot: sourceRoot,
			SourcePaths: []string{
				"replay.md",
			},
			OutDir:  filepath.Join(tmp, "runs"),
			Timeout: 5 * time.Second,
		},
		APIKey:     "test-key",
		BaseURL:    "https://provider.example/v1",
		Model:      "test-model",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("RunTargetOpenAIContractProposal failed: %v", err)
	}
	if result.Accepted != 1 || result.Unsupported != 0 || result.GeneratorKind != TargetContractProposalProviderOpenAICompatible || result.ProviderModel != "test-model" || result.ProviderPromptVersion != TargetContractProposalOpenAISystemPromptVersion || result.GeneratorCommandSHA256 != "" {
		t.Fatalf("unexpected OpenAI-compatible proposal result: %#v", result)
	}
	encoded, err := os.ReadFile(filepath.Join(result.ArtifactDir, result.CandidateSetArtifact))
	if err != nil {
		t.Fatalf("read candidate artifact: %v", err)
	}
	var persisted TargetContractCandidateSet
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatalf("decode candidate artifact: %v", err)
	}
	if persisted.Generator != TargetContractProposalProviderOpenAICompatible {
		t.Fatalf("expected built-in provider provenance, got %#v", persisted)
	}
}

func TestRunTargetOpenAIContractProposalKeepsProviderErrorBodiesOutOfErrors(t *testing.T) {
	client := &http.Client{Transport: contractProposalRoundTripper(func(r *http.Request) (*http.Response, error) {
		return contractProposalHTTPResponse(r, http.StatusBadGateway, "provider secret error body"), nil
	})}

	_, err := RunTargetOpenAIContractProposal(context.Background(), TargetOpenAIContractProposalOptions{
		TargetContractProposalOptions: TargetContractProposalOptions{
			TargetID:   "langgraph-shell-react",
			TaskIDs:    []string{PersistentShellReplayTargetTaskID},
			SourceRoot: writeOpenAIProposalSource(t),
			SourcePaths: []string{
				"replay.md",
			},
			OutDir:  filepath.Join(t.TempDir(), "runs"),
			Timeout: 5 * time.Second,
		},
		APIKey:     "test-key",
		BaseURL:    "https://provider.example/v1",
		Model:      "test-model",
		HTTPClient: client,
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") || strings.Contains(err.Error(), "provider secret error body") {
		t.Fatalf("expected safe HTTP failure, got %v", err)
	}
}

type contractProposalRoundTripper func(*http.Request) (*http.Response, error)

func (fn contractProposalRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func contractProposalHTTPResponse(request *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func writeOpenAIProposalSource(t *testing.T) string {
	t.Helper()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("create source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "replay.md"), []byte("# Replay\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return sourceRoot
}
