package synthesis

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
)

type LangGraphForkPlanConfig struct {
	Model                 string
	ContainerImage        string
	RuntimeRoot           string
	PassiveUnixSocketPath string
}

// PrepareLangGraphForkPlan turns a timestamp-validated native binding into an
// immutable recovery plan. It retains source checkpoint IDs as provenance but
// gives a future fresh runtime only structural coordinates to resolve.
func PrepareLangGraphForkPlan(stateObjective objective.StateObjective, candidate SynthesisCandidate, run objective.ProfileRun, binding LangGraphNativeFrontierBinding, config LangGraphForkPlanConfig) (recovery.LangGraphForkPlan, error) {
	if err := candidate.ValidateFor(stateObjective); err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	if err := run.ValidateFor(stateObjective); err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	if err := binding.Validate(); err != nil {
		return recovery.LangGraphForkPlan{}, err
	}
	if binding.CandidateID != candidate.CandidateID || binding.ProfileRunID != run.ProfileRunID || binding.NativeCheckpointRunID != run.NativeCheckpointRunID || binding.FrontierID == "" || run.AdapterID != recovery.LangGraphForkAdapterID || candidate.AdapterID != recovery.LangGraphForkAdapterID || run.TargetID != candidate.TargetID {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph native binding does not match the candidate/profile recovery identity")
	}
	if strings.TrimSpace(config.Model) == "" || strings.TrimSpace(config.ContainerImage) == "" || strings.TrimSpace(config.RuntimeRoot) == "" || strings.TrimSpace(config.PassiveUnixSocketPath) == "" {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph fork plan requires model, container image, runtime root, and passive Unix socket path")
	}
	runtimeRoot, err := filepath.Abs(strings.TrimSpace(config.RuntimeRoot))
	if err != nil {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("resolve LangGraph runtime root: %w", err)
	}
	plan := recovery.LangGraphForkPlan{
		SchemaVersion:         recovery.LangGraphForkPlanSchema,
		RecordedPlanID:        run.RecordedPlanID,
		AdapterID:             recovery.LangGraphForkAdapterID,
		TargetID:              run.TargetID,
		CandidateID:           candidate.CandidateID,
		Task:                  candidate.Task,
		Model:                 strings.TrimSpace(config.Model),
		ContainerImage:        strings.TrimSpace(config.ContainerImage),
		RuntimeRoot:           runtimeRoot,
		PassiveUnixSocketPath: strings.TrimSpace(config.PassiveUnixSocketPath),
		PassiveObservationID:  "unix-socket-metadata:" + strings.TrimSpace(config.PassiveUnixSocketPath),
		CheckpointCoordinates: map[string]recovery.LangGraphNativeCheckpointCoordinate{
			binding.BeforeProfileCheckpointID: binding.BeforeNativeCoordinate,
			binding.AfterProfileCheckpointID:  binding.AfterNativeCoordinate,
		},
		AgentStateByCheckpoint: map[string]recovery.StatePresence{
			// The timestamp-validated native binding proves the before coordinate
			// was persisted before the effect window and the after coordinate
			// only after it. This is a logical-state projection, not an OS probe.
			binding.BeforeProfileCheckpointID: recovery.StatePresenceAbsent,
			binding.AfterProfileCheckpointID:  recovery.StatePresencePresent,
		},
	}
	if len(plan.CheckpointCoordinates) != 2 {
		return recovery.LangGraphForkPlan{}, fmt.Errorf("LangGraph binding does not preserve two distinct profile checkpoint coordinates")
	}
	return plan, nil
}
