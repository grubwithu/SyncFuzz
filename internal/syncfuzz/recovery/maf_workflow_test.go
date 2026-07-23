package recovery

import "testing"

func TestMAFWorkflowForkPlanRequiresNativeCheckpointBindings(t *testing.T) {
	recorded := RecordedPlan{
		SchemaVersion:        SchemaVersion,
		RecordedPlanID:       "recorded-plan:maf-profile-1",
		AdapterID:            MAFWorkflowForkAdapterID,
		TargetID:             "maf-workflow-checkpoint",
		ExecutionArtifact:    "maf-workflow-fork-plan.json",
		PassiveObservationID: "observe-effect",
	}
	plan := MAFWorkflowForkPlan{
		SchemaVersion:     MAFWorkflowForkPlanSchema,
		RecordedPlanID:    recorded.RecordedPlanID,
		AdapterID:         recorded.AdapterID,
		TargetID:          recorded.TargetID,
		TaskID:            "maf-workflow-checkpoint-continuity",
		PythonCommand:     "python3",
		RunnerPath:        "targets/maf_workflow_checkpoint/run_target.py",
		PreparedWorkspace: "runs/prepared",
		RuntimeRoot:       "runs/forks",
		CheckpointBindings: map[string]string{
			"native-before": "checkpoint-before",
			"native-after":  "checkpoint-after",
		},
	}
	if err := plan.ValidateFor(recorded); err != nil {
		t.Fatalf("validate MAF workflow fork plan: %v", err)
	}
	delete(plan.CheckpointBindings, "native-after")
	if err := plan.ValidateFor(recorded); err == nil {
		t.Fatal("expected missing native checkpoint binding to be rejected")
	}
}

func TestDefaultForkExecutorRegistryRegistersOnlyNativeMAFAdapter(t *testing.T) {
	registry := DefaultForkExecutorRegistry()
	if _, ok := registry.executors[MAFWorkflowForkAdapterID]; !ok {
		t.Fatalf("expected native MAF workflow executor registration: %#v", registry.executors)
	}
	if _, ok := registry.executors["command"]; ok {
		t.Fatalf("legacy command adapter must not have a durable fork executor: %#v", registry.executors)
	}
}

func TestParseMAFWorkflowObservationStateRejectsUnknownValues(t *testing.T) {
	if _, err := parseStatePresence("maybe"); err == nil {
		t.Fatal("expected invalid state presence to be rejected")
	}
	if _, err := parseStateOrigin("maybe"); err == nil {
		t.Fatal("expected invalid state origin to be rejected")
	}
	if _, err := parseEffectMultiplicity("maybe"); err == nil {
		t.Fatal("expected invalid effect multiplicity to be rejected")
	}
}
