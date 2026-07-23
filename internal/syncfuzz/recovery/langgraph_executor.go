package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// LangGraphForkExecutor runs one initial+fresh-resume observation in a new
// constrained container. The wrapper, not the controller, resolves the
// structural coordinate to the newly allocated native checkpoint ID.
type LangGraphForkExecutor struct{}

func NewLangGraphForkExecutor() LangGraphForkExecutor { return LangGraphForkExecutor{} }

func (LangGraphForkExecutor) ExecuteFork(ctx context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
	if request.Plan.AdapterID != LangGraphForkAdapterID {
		return RecoveryObservation{}, fmt.Errorf("LangGraph executor cannot execute adapter %q", request.Plan.AdapterID)
	}
	forkPlan, err := ReadLangGraphForkPlan(request.Plan.ExecutionArtifact)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if err := forkPlan.ValidateFor(request.Plan); err != nil {
		return RecoveryObservation{}, err
	}
	coordinate, ok := forkPlan.CheckpointCoordinates[request.Query.CheckpointID]
	if !ok {
		return RecoveryObservation{}, fmt.Errorf("LangGraph fork plan has no coordinate for query checkpoint %q", request.Query.CheckpointID)
	}
	runtimeRoot, err := filepath.Abs(forkPlan.RuntimeRoot)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("resolve LangGraph runtime root: %w", err)
	}
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		return RecoveryObservation{}, fmt.Errorf("create LangGraph runtime root: %w", err)
	}
	workspace, err := os.MkdirTemp(runtimeRoot, "syncfuzz-langgraph-fork-")
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("allocate LangGraph runtime workspace: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "prompt.txt"), []byte(forkPlan.Task+"\n"), 0o644); err != nil {
		return RecoveryObservation{}, err
	}
	coordinatePath := filepath.Join(workspace, "native-coordinate.json")
	encoded, err := json.Marshal(coordinate)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if err := os.WriteFile(coordinatePath, append(encoded, '\n'), 0o644); err != nil {
		return RecoveryObservation{}, err
	}
	runtimeID := "langgraph-fork-" + filepath.Base(workspace)
	observationPath := filepath.Join(workspace, "langgraph-recovery-observation.json")
	args := []string{"run", "--rm", "--name", "syncfuzz-" + runtimeID, "--pids-limit", "128", "--memory", "256m", "--cpus", "1", "--security-opt", "no-new-privileges", "--cap-drop", "ALL", "--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()), "-v", workspace + ":/workspace", "-w", "/workspace", "-e", "LANGCHAIN_MODEL=" + forkPlan.Model}
	for _, key := range []string{"OPENAI_API_KEY", "OPENAI_ADMIN_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY"} {
		if value := os.Getenv(key); value != "" {
			args = append(args, "-e", key+"="+value)
		}
	}
	args = append(args, forkPlan.ContainerImage, "python3", "/opt/syncfuzz-langgraph/run_target.py", "--workspace", "/workspace", "--prompt-file", "/workspace/prompt.txt", "--thread-id", runtimeID, "--execution-policy", "host", "--checkpoint-backend", "disk", "--process-mode", "split-process", "--checkpoint-coordinate-file", "/workspace/native-coordinate.json", "--passive-fork-observe", "--passive-unix-socket-path", forkPlan.PassiveUnixSocketPath, "--runtime-instance-id", runtimeID, "--recovery-observation-artifact", "/workspace/langgraph-recovery-observation.json", "--require-tool-use")
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("run LangGraph recovery container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(observationPath)
	if err != nil {
		return RecoveryObservation{}, fmt.Errorf("read LangGraph recovery observation: %w", err)
	}
	var artifact struct {
		RuntimeInstanceID              string   `json:"runtime_instance_id"`
		RuntimeRecreated               bool     `json:"runtime_recreated"`
		RestoredCheckpointID           string   `json:"restored_checkpoint_id"`
		RestoredCheckpointMessageCount int      `json:"restored_checkpoint_message_count"`
		RestoredCheckpointNext         []string `json:"restored_checkpoint_next"`
		PassiveUnixSocket              struct {
			BeforeFork           map[string]any `json:"before_fork"`
			AfterFork            map[string]any `json:"after_fork"`
			SameEndpointIdentity bool           `json:"same_endpoint_identity"`
		} `json:"passive_unix_socket"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		return RecoveryObservation{}, fmt.Errorf("decode LangGraph recovery observation: %w", err)
	}
	if artifact.RuntimeInstanceID != runtimeID || !artifact.RuntimeRecreated || artifact.RestoredCheckpointID == "" {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation does not prove fresh native checkpoint restore")
	}
	if artifact.RestoredCheckpointMessageCount != coordinate.MessageCount || !sameStrings(artifact.RestoredCheckpointNext, coordinate.Next) {
		return RecoveryObservation{}, fmt.Errorf("LangGraph recovery observation did not restore the planned native state shape")
	}
	osState := StatePresenceAbsent
	if socketPresent(artifact.PassiveUnixSocket.AfterFork) {
		osState = StatePresencePresent
	}
	origin := StateOriginNone
	if osState == StatePresencePresent && artifact.PassiveUnixSocket.SameEndpointIdentity {
		origin = StateOriginResidual
	} else if osState == StatePresencePresent {
		origin = StateOriginUnknown
	}
	agentState := forkPlan.AgentStateByCheckpoint[request.Query.CheckpointID]
	return RecoveryObservation{SchemaVersion: ExecutionSchemaVersion, QueryID: request.Query.QueryID, SeedID: request.Query.SeedID, Boundary: request.Query.Boundary, CheckpointID: request.Query.CheckpointID, RecordedPlanID: request.Query.RecordedPlanID, PassiveObservationID: request.Query.PassiveObservationID, RuntimeInstanceID: runtimeID, AgentState: agentState, OSState: osState, OSStateOrigin: origin, EffectMultiplicity: EffectMultiplicityUnknown, Evidence: []string{"LangGraph fresh container: " + runtimeID, "native coordinate resolved to: " + artifact.RestoredCheckpointID, "timestamp-validated logical state: " + string(agentState), "passive observation artifact: " + observationPath}}, nil
}

func socketPresent(value map[string]any) bool {
	present, _ := value["is_unix_socket"].(bool)
	return present
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

var _ ForkExecutor = LangGraphForkExecutor{}
