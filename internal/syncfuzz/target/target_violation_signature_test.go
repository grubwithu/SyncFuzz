package target

import "testing"

func TestTargetScenariosCarryNormalizedViolationSignatures(t *testing.T) {
	scenarios := TargetScenarios()
	if len(scenarios) == 0 {
		t.Fatal("expected built-in scenarios")
	}
	for _, scenario := range scenarios {
		signature, err := NormalizeTargetViolationSignature(scenario.ViolationSignature)
		if err != nil {
			t.Fatalf("scenario %q has invalid violation signature: %v", scenario.ScenarioID, err)
		}
		if signature.SignatureID == "" || signature.SignatureID != scenario.ViolationSignature.SignatureID {
			t.Fatalf("scenario %q has non-canonical violation signature: %#v", scenario.ScenarioID, scenario.ViolationSignature)
		}
	}
}

func TestTargetViolationSignatureClassifiesRepresentativeAOQueries(t *testing.T) {
	tests := []struct {
		taskID      string
		relation    TargetViolationRelation
		resource    TargetViolationResourceClass
		boundary    TargetViolationLifecycleBoundary
		mechanism   TargetViolationPersistenceMechanism
		consequence TargetViolationConsequence
	}{
		{
			taskID:      LongDelayTargetTaskID,
			relation:    TargetViolationReorderedDelayed,
			resource:    TargetViolationProcess,
			boundary:    TargetViolationBoundaryCommandReturn,
			mechanism:   TargetViolationDelayedExecution,
			consequence: TargetViolationObservation,
		},
		{
			taskID:      PersistentShellForkTargetTaskID,
			relation:    TargetViolationResidual,
			resource:    TargetViolationExecutionContext,
			boundary:    TargetViolationBoundaryCheckpointFork,
			mechanism:   TargetViolationPersistentShell,
			consequence: TargetViolationObservation,
		},
		{
			taskID:      OpenFDResidueForkTargetTaskID,
			relation:    TargetViolationResidual,
			resource:    TargetViolationHandleCapability,
			boundary:    TargetViolationBoundaryCheckpointFork,
			mechanism:   TargetViolationFDInheritance,
			consequence: TargetViolationObservation,
		},
		{
			taskID:      UnixListenerResidueForkTargetTaskID,
			relation:    TargetViolationResidual,
			resource:    TargetViolationIPCEndpoint,
			boundary:    TargetViolationBoundaryCheckpointFork,
			mechanism:   TargetViolationDescendantSurvival,
			consequence: TargetViolationObservation,
		},
		{
			taskID:      MAFWorkflowExternalReplayTargetTaskID,
			relation:    TargetViolationDuplicated,
			resource:    TargetViolationExternalEffect,
			boundary:    TargetViolationBoundaryCheckpointRestore,
			mechanism:   TargetViolationRuntimeReexecution,
			consequence: TargetViolationDuplicateOperation,
		},
	}
	for _, test := range tests {
		t.Run(test.taskID, func(t *testing.T) {
			scenario, ok := TargetScenarioByTaskID(test.taskID)
			if !ok {
				t.Fatalf("expected scenario %q", test.taskID)
			}
			signature := scenario.ViolationSignature
			if signature.LifecycleBoundary != test.boundary || !targetViolationTestContains(signature.Relations, test.relation) || !targetViolationTestContains(signature.ResourceClasses, test.resource) || !targetViolationTestContains(signature.PersistenceMechanisms, test.mechanism) || !targetViolationTestContains(signature.Consequences, test.consequence) {
				t.Fatalf("unexpected violation signature: %#v", signature)
			}
		})
	}
}

func TestGeneratedScenariosAreReclassifiedAfterMutation(t *testing.T) {
	scenario, _, err := GeneratedTrustedActionActivationSubstitution()
	if err != nil {
		t.Fatalf("GeneratedTrustedActionActivationSubstitution failed: %v", err)
	}
	if scenario == nil {
		t.Fatal("expected generated scenario")
	}
	normalized, err := NormalizeTargetScenarioInfo(scenario)
	if err != nil {
		t.Fatalf("normalize generated scenario: %v", err)
	}
	signature := normalized.ViolationSignature
	if !targetViolationTestContains(signature.ResourceClasses, TargetViolationIPCEndpoint) || !targetViolationTestContains(signature.Consequences, TargetViolationCrossBranchInterference) || !targetViolationTestContains(signature.Consequences, TargetViolationStateCorruption) {
		t.Fatalf("expected trusted-action IPC reclassification, got %#v", signature)
	}
}

func TestTargetViolationSignatureForTaskDoesNotInventCustomTaskLabels(t *testing.T) {
	if signature := TargetViolationSignatureForTask("custom-unclassified-task", nil); signature != nil {
		t.Fatalf("expected no signature for unknown custom task, got %#v", signature)
	}
	signature := TargetViolationSignatureForTask(UnixListenerResidueForkTargetTaskID, nil)
	if signature == nil || !targetViolationTestContains(signature.ResourceClasses, TargetViolationIPCEndpoint) {
		t.Fatalf("expected built-in IPC signature, got %#v", signature)
	}
}

func targetViolationTestContains[T comparable](values []T, expected T) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
