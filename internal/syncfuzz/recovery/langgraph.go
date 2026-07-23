package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	LangGraphForkAdapterID          = "langgraph"
	LangGraphForkPlanSchema         = "syncfuzz.langgraph-fork-plan.v1"
	LangGraphNativeCoordinateSchema = "syncfuzz.langgraph-native-coordinate.v1"
)

// LangGraphNativeCheckpointCoordinate is the complete structural shape that
// must resolve to one newly generated native ID in a fresh initial runtime.
// SourceCheckpointID is provenance only and is never supplied to that runtime.
type LangGraphNativeCheckpointCoordinate struct {
	SchemaVersion      string   `json:"schema_version"`
	SourceCheckpointID string   `json:"source_checkpoint_id"`
	HistoryIndex       int      `json:"history_index"`
	MessageCount       int      `json:"message_count"`
	Next               []string `json:"next"`
}

func (c LangGraphNativeCheckpointCoordinate) Validate() error {
	if c.SchemaVersion != LangGraphNativeCoordinateSchema || strings.TrimSpace(c.SourceCheckpointID) == "" || c.HistoryIndex < 0 || c.MessageCount < 0 {
		return fmt.Errorf("LangGraph native checkpoint coordinate is incomplete")
	}
	for _, node := range c.Next {
		if strings.TrimSpace(node) == "" {
			return fmt.Errorf("LangGraph native checkpoint coordinate has an empty next node")
		}
	}
	return nil
}

// LangGraphForkPlan freezes the source task, model identity, container image,
// passive probe and structural native coordinates. Credentials deliberately do
// not appear here: the future executor receives them only from its process
// environment.
type LangGraphForkPlan struct {
	SchemaVersion          string                                         `json:"schema_version"`
	RecordedPlanID         string                                         `json:"recorded_plan_id"`
	AdapterID              string                                         `json:"adapter_id"`
	TargetID               string                                         `json:"target_id"`
	CandidateID            string                                         `json:"candidate_id"`
	Task                   string                                         `json:"task"`
	Model                  string                                         `json:"model"`
	ContainerImage         string                                         `json:"container_image"`
	RuntimeRoot            string                                         `json:"runtime_root"`
	PassiveUnixSocketPath  string                                         `json:"passive_unix_socket_path"`
	PassiveObservationID   string                                         `json:"passive_observation_id"`
	CheckpointCoordinates  map[string]LangGraphNativeCheckpointCoordinate `json:"checkpoint_coordinates"`
	AgentStateByCheckpoint map[string]StatePresence                       `json:"agent_state_by_checkpoint"`
}

func (p LangGraphForkPlan) ValidateFor(plan RecordedPlan) error {
	if p.SchemaVersion != LangGraphForkPlanSchema || p.RecordedPlanID != plan.RecordedPlanID || p.AdapterID != plan.AdapterID || p.TargetID != plan.TargetID {
		return fmt.Errorf("LangGraph fork plan does not match recorded plan %q", plan.RecordedPlanID)
	}
	if p.AdapterID != LangGraphForkAdapterID || strings.TrimSpace(p.CandidateID) == "" || strings.TrimSpace(p.Task) == "" || strings.TrimSpace(p.Model) == "" || strings.TrimSpace(p.ContainerImage) == "" || strings.TrimSpace(p.RuntimeRoot) == "" || strings.TrimSpace(p.PassiveUnixSocketPath) == "" || p.PassiveObservationID != plan.PassiveObservationID {
		return fmt.Errorf("LangGraph fork plan requires candidate, task, model, image, runtime root, and passive Unix socket path")
	}
	if len(p.CheckpointCoordinates) != 2 {
		return fmt.Errorf("LangGraph fork plan requires exactly two checkpoint coordinates")
	}
	if len(p.AgentStateByCheckpoint) != len(p.CheckpointCoordinates) {
		return fmt.Errorf("LangGraph fork plan requires one logical-state projection per checkpoint")
	}
	for profileCheckpoint, coordinate := range p.CheckpointCoordinates {
		if strings.TrimSpace(profileCheckpoint) == "" {
			return fmt.Errorf("LangGraph fork plan has an empty profile checkpoint coordinate")
		}
		if err := coordinate.Validate(); err != nil {
			return err
		}
		state, ok := p.AgentStateByCheckpoint[profileCheckpoint]
		if !ok || (state != StatePresenceAbsent && state != StatePresencePresent) {
			return fmt.Errorf("LangGraph fork plan has no deterministic logical-state projection for checkpoint %q", profileCheckpoint)
		}
	}
	return nil
}

func ReadLangGraphForkPlan(path string) (LangGraphForkPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LangGraphForkPlan{}, fmt.Errorf("read LangGraph fork plan %s: %w", path, err)
	}
	var plan LangGraphForkPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return LangGraphForkPlan{}, fmt.Errorf("decode LangGraph fork plan %s: %w", path, err)
	}
	return plan, nil
}

func WriteLangGraphForkPlan(path string, plan LangGraphForkPlan) error {
	if plan.SchemaVersion != LangGraphForkPlanSchema {
		return fmt.Errorf("unsupported LangGraph fork plan schema %q", plan.SchemaVersion)
	}
	return writeRecoveryJSON(path, plan)
}
