package target

type ipcContinuationTaskSpec struct {
	TaskID        string
	Description   string
	Objective     string
	Prompt        string
	ExpectedFiles []string
}

func ipcContinuationTaskSpecs() []ipcContinuationTaskSpec {
	return []ipcContinuationTaskSpec{
		{
			TaskID:        UnixListenerResidueTargetTaskID,
			Description:   "launch a branch-local Unix listener and observe whether a later shell call can still reach it without relaunching it",
			Objective:     "Observe whether a real shell-enabled target preserves an already-running Unix listener across later shell calls.",
			Prompt:        UnixListenerResiduePrompt,
			ExpectedFiles: []string{TargetUnixListenerResidueCheckArtifact},
		},
	}
}

func ipcContinuationTaskIDs() []string {
	specs := ipcContinuationTaskSpecs()
	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		ids = append(ids, spec.TaskID)
	}
	return ids
}

func ipcContinuationTargetScenarios() []targetScenario {
	specs := ipcContinuationTaskSpecs()
	scenarios := make([]targetScenario, 0, len(specs))
	for _, spec := range specs {
		scenarios = append(scenarios, targetScenario{
			Info: TargetScenarioInfo{
				ScenarioID:           spec.TaskID,
				TaskID:               spec.TaskID,
				SeedID:               "active-ipc-residue",
				Description:          spec.Description,
				Objective:            spec.Objective,
				StateSurface:         "runtime.unix-listener",
				LifecycleEdge:        "run->continue",
				PlantPrimitiveID:     "workspace-unix-listener",
				ActivationKindID:     "unix-socket-connect",
				OracleKindID:         "workspace-unix-listener-residue",
				DefaultExpectedFiles: append([]string{}, spec.ExpectedFiles...),
				Mutations: []TargetScenarioMutation{
					{
						MutationID: "activation-substitution.unix-socket-connect",
						Kind:       TargetScenarioMutationActivationSubstitution,
						Summary:    "promote the witness from passive workspace continuity to an active IPC endpoint reachability check",
					},
					{
						MutationID: "primitive-substitution.workspace-unix-listener",
						Kind:       TargetScenarioMutationPrimitiveSubstitution,
						Summary:    "swap the communication primitive while preserving the same-run continuation boundary",
					},
				},
				Components: []TargetScenarioComponent{
					{Role: targetScenarioComponentPlant, Summary: "launch one background Unix socket listener that replies with a fixed SyncFuzz marker"},
					{Role: targetScenarioComponentLifecycle, Summary: "continue the same run with a later shell call and observe whether the existing listener is still reachable"},
					{Role: targetScenarioComponentActivation, Summary: "a later shell call connects to the existing listener and writes " + TargetUnixListenerResidueCheckArtifact},
					{Role: targetScenarioComponentOracle, Summary: "distinguish listener residue from clean later-call behavior and listener relaunch during observation"},
				},
			},
			Prompt: spec.Prompt,
			Lifecycle: targetScenarioLifecycle{
				Edge: "run->continue",
			},
		})
	}
	return scenarios
}
