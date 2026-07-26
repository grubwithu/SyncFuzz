package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

const (
	LangGraphRuntimeContractSchema = "syncfuzz.langgraph-runtime-contract.v1"
	LangGraphRuntimeTargetID       = "langgraph-shell-react"
	LangGraphRunnerProtocol        = "syncfuzz.langgraph-runner.v1"
)

var requiredLangGraphRuntimeCapabilities = []string{
	"durable-disk-checkpoints-v1",
	"exact-checkpoint-restore-v1",
	"passive-unix-socket-observer-v1",
	"passive-workspace-file-observer-v1",
}

const langGraphContinuationRuntimeCapability = "continuation-user-turn-v1"

// LangGraphRuntimeContract is the compatibility evidence returned by the
// image-owned runner before profiling and before every recovery query. ImageID
// pins the mutable Docker tag to the concrete image used by the source lease.
type LangGraphRuntimeContract struct {
	SchemaVersion  string   `json:"schema_version"`
	TargetID       string   `json:"target_id"`
	RunnerProtocol string   `json:"runner_protocol"`
	Capabilities   []string `json:"capabilities"`
	ImageID        string   `json:"image_id"`
}

func (c LangGraphRuntimeContract) Validate() error {
	if c.SchemaVersion != LangGraphRuntimeContractSchema || c.TargetID != LangGraphRuntimeTargetID || c.RunnerProtocol != LangGraphRunnerProtocol || !strings.HasPrefix(c.ImageID, "sha256:") {
		return fmt.Errorf("LangGraph runtime contract is incomplete")
	}
	capabilities := append([]string(nil), c.Capabilities...)
	sort.Strings(capabilities)
	if len(capabilities) != len(requiredLangGraphRuntimeCapabilities) && len(capabilities) != len(requiredLangGraphRuntimeCapabilities)+1 {
		return fmt.Errorf("LangGraph runtime contract has an unsupported capability set")
	}
	required := make(map[string]struct{}, len(requiredLangGraphRuntimeCapabilities))
	for _, capability := range requiredLangGraphRuntimeCapabilities {
		required[capability] = struct{}{}
	}
	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("LangGraph runtime contract has an unsupported capability set")
		}
		seen[capability] = struct{}{}
		if capability == langGraphContinuationRuntimeCapability {
			continue
		}
		if _, ok := required[capability]; !ok {
			return fmt.Errorf("LangGraph runtime contract has an unsupported capability set")
		}
	}
	for _, capability := range requiredLangGraphRuntimeCapabilities {
		if _, ok := seen[capability]; !ok {
			return fmt.Errorf("LangGraph runtime contract has an unsupported capability set")
		}
	}
	return nil
}

// SupportsContinuation reports whether this exact runner image advertises the
// capability required to deliver one frozen post-restore user turn.
func (c LangGraphRuntimeContract) SupportsContinuation() bool {
	for _, capability := range c.Capabilities {
		if capability == langGraphContinuationRuntimeCapability {
			return true
		}
	}
	return false
}

func (c LangGraphRuntimeContract) Matches(other LangGraphRuntimeContract) bool {
	capabilities := append([]string(nil), c.Capabilities...)
	otherCapabilities := append([]string(nil), other.Capabilities...)
	sort.Strings(capabilities)
	sort.Strings(otherCapabilities)
	return c.SchemaVersion == other.SchemaVersion && c.TargetID == other.TargetID && c.RunnerProtocol == other.RunnerProtocol && c.ImageID == other.ImageID && sameStrings(capabilities, otherCapabilities)
}

// VerifyLangGraphRuntime queries the runner inside the exact Docker image.
// This checks the executable interface rather than trusting an image tag or a
// Dockerfile label that might no longer match the copied runner script.
func VerifyLangGraphRuntime(ctx context.Context, image string) (LangGraphRuntimeContract, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return LangGraphRuntimeContract{}, fmt.Errorf("LangGraph runtime verification requires a container image")
	}
	imageIDOutput, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", image).CombinedOutput()
	if err != nil {
		return LangGraphRuntimeContract{}, fmt.Errorf("inspect LangGraph runtime image %q: %w: %s", image, err, strings.TrimSpace(string(imageIDOutput)))
	}
	imageID := strings.TrimSpace(string(imageIDOutput))
	// Run by immutable ID after resolving the user-supplied reference. A tag
	// must not be able to move between inspection and the runner self-check.
	output, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "none", "--read-only", "--entrypoint", "python3", imageID, "/opt/syncfuzz-langgraph/run_target.py", "--runtime-contract").CombinedOutput()
	if err != nil {
		return LangGraphRuntimeContract{}, fmt.Errorf("query LangGraph runtime contract for image %q: %w: %s", image, err, strings.TrimSpace(string(output)))
	}
	return ParseLangGraphRuntimeContract(output, imageID)
}

func ParseLangGraphRuntimeContract(data []byte, imageID string) (LangGraphRuntimeContract, error) {
	var contract LangGraphRuntimeContract
	if err := json.Unmarshal(data, &contract); err != nil {
		return LangGraphRuntimeContract{}, fmt.Errorf("decode LangGraph runtime contract: %w", err)
	}
	contract.ImageID = strings.TrimSpace(imageID)
	if err := contract.Validate(); err != nil {
		return LangGraphRuntimeContract{}, err
	}
	sort.Strings(contract.Capabilities)
	return contract, nil
}
