package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/cases"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/corpus"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/observation"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/scheduler"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

const version = "0.1.0-mvp"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "list":
		list()
	case "fault-plans":
		faultPlans()
	case "timing-profiles":
		timingProfiles()
	case "primitives":
		primitives()
	case "matrix":
		matrix(os.Args[2:])
	case "run":
		run(os.Args[2:])
	case "pair":
		pair(os.Args[2:])
	case "suite":
		suite(os.Args[2:])
	case "campaign":
		campaign(os.Args[2:])
	case "target":
		runTarget(os.Args[2:])
	case "corpus":
		runCorpus(os.Args[2:])
	case "replay":
		replay(os.Args[2:])
	case "version":
		fmt.Println(version)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `SyncFuzz %s

Usage:
  syncfuzz list
  syncfuzz fault-plans
  syncfuzz timing-profiles
  syncfuzz primitives [--include-planned]
  syncfuzz matrix [--cases orphan-process,branch-leakage] [--timing baseline,tight,wide] [--include-planned]
  syncfuzz run --case orphan-process [--out runs] [--delay 1500ms] [--fault-plan <id>] [--primitive delayed-write] [--timing baseline] [--role fault] [--env local] [--container-image ubuntu:latest]
  syncfuzz pair --case orphan-process [--out runs] [--delay 1500ms] [--fault-plan <id>] [--primitive delayed-write] [--timing baseline] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case action-replay [--out runs] [--mock-url http://127.0.0.1:8910] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case authority-resurrection [--out runs] [--mock-url http://127.0.0.1:8910] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case persistent-shell-poisoning [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case partial-filesystem-rollback [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case branch-leakage [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz suite [--out runs] [--repeat 1] [--corpus corpus] [--cases orphan-process,branch-leakage] [--timing baseline] [--differential] [--env local] [--container-image ubuntu:latest]
  syncfuzz suite --matrix [--out runs] [--repeat 1] [--corpus corpus] [--cases orphan-process] [--timing baseline,tight,wide] [--feedback-from matrix-result.json] [--candidate-limit 5] [--differential] [--env local] [--container-image ubuntu:latest]
  syncfuzz campaign [--rounds 2] [--candidate-limit 3] [--cases action-replay] [--timing baseline,tight,wide] [--feedback-from matrix-result.json] [--out runs] [--corpus corpus] [--env local] [--container-image ubuntu:latest]
  syncfuzz target list
  syncfuzz target tasks
  syncfuzz target seeds
  syncfuzz target scenarios
  syncfuzz target groups
  syncfuzz target prompt-profiles
  syncfuzz target prompt-variants
  syncfuzz target footprint --run runs/<target-run-id> [--out resource-footprint.json]
  syncfuzz target plan-probes --footprint resource-footprint.json [--out observation-plan.json]
  syncfuzz target matrix [--target langgraph-shell-react] [--task orphan-process] [--tasks orphan-process-long-delay,persistent-shell-poisoning] [--seed shell-path-residue] [--seeds workspace-object-residue-fork] [--group workspace-residue] [--groups phase5a-baseline] [--prompt-profile baseline] [--prompt-profiles all]
  syncfuzz target minimize --from runs/target-suite-<id>/target-suite-result.json [--execute] [--candidate-limit 1] [--max-trials 32] [--fidelity exact|semantic|impact] [--out runs]
  syncfuzz target run [--command '<agent command>' | --command-file examples/target-commands/orphan-process.sh] [--target local-agent] [--task orphan-process|orphan-process-long-delay|persistent-shell-poisoning|persistent-shell-poisoning-replay|persistent-shell-poisoning-fork|file-residue-fork|directory-residue-fork|delete-residue-fork|symlink-residue-fork] [--prompt-profile baseline|workflow|audit] [--prompt-file task.md] [--expect-files late-effect] [--timeout 2m] [--observe-delay 500ms] [--late-observe-delay 7s] [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz target run [--target maf-github-copilot-shell] [--task orphan-process] [--command-file examples/target-commands/maf-github-copilot-shell.sh] [--observe-delay 500ms] [--out runs]
  syncfuzz target suite [--command '<agent command>' | --command-file examples/target-commands/orphan-process.sh] [--target local-agent] [--task orphan-process] [--tasks orphan-process,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,directory-residue-fork,delete-residue-fork,symlink-residue-fork] [--seed shell-path-residue] [--seeds workspace-object-residue-fork] [--group workspace-residue] [--groups phase5a-baseline] [--prompt-profile baseline] [--prompt-profiles baseline,workflow,audit] [--matrix] [--selection-policy explore|feedback|fixed|random] [--random-seed 1] [--feedback-from target-matrix-result.json] [--candidate-limit 3] [--repeat 3] [--timeout 2m] [--observe-delay 500ms] [--late-observe-delay 7s] [--out runs] [--corpus corpus] [--env local] [--container-image ubuntu:latest]
  syncfuzz target campaign [--command '<agent command>' | --command-file examples/target-commands/orphan-process.sh] [--target local-agent] [--tasks orphan-process-long-delay,persistent-shell-poisoning] [--seed shell-path-residue] [--group phase5a-baseline] [--prompt-profiles baseline,workflow,audit] [--rounds 2] [--selection-policy explore|feedback|fixed|random] [--random-seed 1] [--candidate-limit 3] [--repeat 1] [--min-coverage-gain-score 0] [--max-stagnant-rounds 0] [--auto-pivot] [--timeout 2m] [--observe-delay 500ms] [--late-observe-delay 7s] [--out runs] [--corpus corpus] [--env local] [--container-image ubuntu:latest]
  syncfuzz corpus list [--corpus corpus] [--limit 20]
  syncfuzz corpus analyze [--corpus corpus] [--limit 0] [--verification runs/verify-<id>/verification-result.json]
  syncfuzz corpus show --id <entry_id> [--corpus corpus]
  syncfuzz corpus verify [--corpus corpus] [--out runs] [--limit 0] [--env local] [--container-image ubuntu:latest]
  syncfuzz replay --id <entry_id> [--corpus corpus] [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz version

`, version)
}

func list() {
	for _, c := range core.Cases() {
		fmt.Printf("%-30s %s\n", c.Name, c.Description)
	}
}

func faultPlans() {
	fmt.Printf("%-58s %-28s %-5s %-28s %s\n", "id", "case", "phase", "impact", "fault")
	for _, plan := range core.FaultPlans() {
		fmt.Printf("%-58s %-28s %-5s %-28s %s\n",
			plan.ID,
			plan.CaseName,
			plan.InjectPhase,
			plan.ExpectedImpact,
			plan.Fault,
		)
	}
}

func timingProfiles() {
	fmt.Printf("%-12s %-16s %-16s %-14s %s\n", "id", "recovery", "orphan_child", "replay", "description")
	for _, profile := range core.TimingProfiles() {
		recoveryDelay := profile.RecoveryDelay
		if recoveryDelay == "" {
			recoveryDelay = "<--delay>"
		}
		fmt.Printf("%-12s %-16s %-16s %-14s %s\n",
			profile.ProfileID,
			recoveryDelay,
			profile.OrphanChildDelay,
			profile.ReplayDelay,
			profile.Description,
		)
	}
}

func primitives() {
	fs := flag.NewFlagSet("primitives", flag.ExitOnError)
	includePlanned := fs.Bool("include-planned", false, "include planned primitives that are not executable yet")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}

	fmt.Printf("%-30s %-12s %-7s %-28s %s\n", "id", "category", "ready", "cases", "description")
	for _, primitive := range core.MutationPrimitives() {
		if !*includePlanned && !primitive.Implemented {
			continue
		}
		fmt.Printf("%-30s %-12s %-7t %-28s %s\n",
			primitive.ID,
			primitive.Category,
			primitive.Implemented,
			strings.Join(primitive.CaseNames, ","),
			primitive.Description,
		)
	}
}

func matrix(args []string) {
	fs := flag.NewFlagSet("matrix", flag.ExitOnError)
	caseList := fs.String("cases", "", "comma-separated testcase names; defaults to all")
	timingList := fs.String("timing", "", "comma-separated timing profile ids; defaults to all")
	includePlanned := fs.Bool("include-planned", false, "include planned primitive candidates")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := scheduler.BuildScheduleMatrix(scheduler.MatrixOptions{
		Cases:            splitCases(*caseList),
		TimingProfileIDs: splitCSV(*timingList),
		IncludePlanned:   *includePlanned,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz matrix failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("schema: %s\n", result.SchemaVersion)
	fmt.Printf("cases: %s\n", strings.Join(result.Cases, ","))
	fmt.Printf("timing_profiles: %s\n", strings.Join(result.TimingProfiles, ","))
	fmt.Printf("include_planned: %t\n", result.IncludePlanned)
	fmt.Printf("total_candidates: %d\n", result.TotalCandidates)
	fmt.Printf("%-58s %-28s %-30s %-8s %s\n", "candidate_id", "case", "primitive", "timing", "impact")
	for _, candidate := range result.Candidates {
		fmt.Printf("%-58s %-28s %-30s %-8s %s\n",
			candidate.CandidateID,
			candidate.CaseName,
			candidate.PrimitiveID,
			candidate.TimingProfileID,
			candidate.ExpectedImpact,
		)
	}
}

func run(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	caseName := fs.String("case", "orphan-process", "testcase to execute")
	outDir := fs.String("out", "runs", "directory for run artifacts")
	delay := fs.Duration("delay", 1500*time.Millisecond, "delay before the recovery snapshot")
	workspace := fs.String("workspace", "", "optional workspace; defaults to runs/<run_id>/workspace")
	mockURL := fs.String("mock-url", "", "optional EffectServer/AuthorityServer base URL")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	faultPlanID := fs.String("fault-plan", "", "fault plan id; defaults to the testcase known-answer plan")
	primitiveID := fs.String("primitive", "", "optional mutation primitive id")
	timingProfileID := fs.String("timing", "", "timing profile id; defaults to baseline")
	runRole := fs.String("role", core.RunRoleFault, "run role: fault or control")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	opts := core.RunOptions{
		CaseName:        *caseName,
		OutDir:          *outDir,
		Workspace:       *workspace,
		Delay:           *delay,
		MockURL:         *mockURL,
		EnvKind:         *envKind,
		ContainerImage:  *containerImage,
		FaultPlanID:     *faultPlanID,
		PrimitiveID:     *primitiveID,
		TimingProfileID: *timingProfileID,
		RunRole:         *runRole,
	}

	// The CLI is intentionally thin: all testcase semantics live in the
	// syncfuzz package so future adapters can reuse the same runner directly.
	result, err := cases.Run(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz run failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("run_id: %s\n", result.RunID)
	fmt.Printf("case: %s\n", result.CaseName)
	fmt.Printf("run_role: %s\n", result.RunRole)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	printFaultPlan(result.FaultPlanID)
	printPrimitive(result.PrimitiveID)
	printTimingProfile(result.TimingProfileID)
	fmt.Printf("confirmed: %t\n", result.Confirmed)
	fmt.Printf("signature: %s\n", result.Signature.String())
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func pair(args []string) {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	caseName := fs.String("case", "orphan-process", "testcase to execute as a control/fault pair")
	outDir := fs.String("out", "runs", "directory for pair artifacts")
	delay := fs.Duration("delay", 1500*time.Millisecond, "delay passed through to testcase runs")
	mockURL := fs.String("mock-url", "", "optional EffectServer/AuthorityServer base URL")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	faultPlanID := fs.String("fault-plan", "", "fault plan id; defaults to the testcase known-answer plan")
	primitiveID := fs.String("primitive", "", "optional mutation primitive id")
	timingProfileID := fs.String("timing", "", "timing profile id; defaults to baseline")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := scheduler.RunPair(context.Background(), scheduler.PairOptions{
		CaseName:        *caseName,
		OutDir:          *outDir,
		Delay:           *delay,
		MockURL:         *mockURL,
		EnvKind:         *envKind,
		ContainerImage:  *containerImage,
		FaultPlanID:     *faultPlanID,
		PrimitiveID:     *primitiveID,
		TimingProfileID: *timingProfileID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz pair failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("pair_id: %s\n", result.PairID)
	fmt.Printf("case: %s\n", result.CaseName)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	printFaultPlan(result.FaultPlanID)
	printPrimitive(result.PrimitiveID)
	printTimingProfile(result.TimingProfileID)
	fmt.Printf("control_run: %s confirmed=%t\n", result.Control.RunID, result.Control.Confirmed)
	fmt.Printf("fault_run: %s confirmed=%t\n", result.Fault.RunID, result.Fault.Confirmed)
	fmt.Printf("differential: %t\n", result.Verdict.Differential)
	fmt.Printf("security_relevant: %t\n", result.Verdict.SecurityRelevant)
	fmt.Printf("reason: %s\n", result.Verdict.Reason)
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func suite(args []string) {
	fs := flag.NewFlagSet("suite", flag.ExitOnError)
	outDir := fs.String("out", "runs", "directory for suite artifacts")
	repeat := fs.Int("repeat", 1, "number of times to run each testcase")
	caseList := fs.String("cases", "", "comma-separated testcase names; defaults to all")
	delay := fs.Duration("delay", 1500*time.Millisecond, "delay passed through to testcase runs")
	mockURL := fs.String("mock-url", "", "optional EffectServer/AuthorityServer base URL")
	corpusDir := fs.String("corpus", "corpus", "directory for interesting discovery corpus; empty disables corpus output")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	differential := fs.Bool("differential", false, "run each testcase as a control/fault pair")
	timingProfileID := fs.String("timing", "", "timing profile id; in matrix mode accepts a comma-separated list")
	matrixMode := fs.Bool("matrix", false, "run the deterministic Phase 4 schedule matrix")
	feedbackFrom := fs.String("feedback-from", "", "previous matrix-result.json used to rank matrix candidates")
	candidateLimit := fs.Int("candidate-limit", 0, "maximum matrix candidates to execute after feedback ranking; 0 means all")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	var timingProfileIDs []string
	timingProfile := *timingProfileID
	if *matrixMode {
		timingProfileIDs = splitCSV(*timingProfileID)
		timingProfile = ""
	}

	result, err := scheduler.RunSuite(context.Background(), scheduler.SuiteOptions{
		OutDir:           *outDir,
		Repeat:           *repeat,
		Cases:            splitCases(*caseList),
		Delay:            *delay,
		MockURL:          *mockURL,
		CorpusDir:        *corpusDir,
		EnvKind:          *envKind,
		ContainerImage:   *containerImage,
		Differential:     *differential,
		TimingProfileID:  timingProfile,
		Matrix:           *matrixMode,
		TimingProfileIDs: timingProfileIDs,
		FeedbackFrom:     *feedbackFrom,
		CandidateLimit:   *candidateLimit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz suite failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("suite_id: %s\n", result.SuiteID)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	printTimingProfile(result.TimingProfileID)
	fmt.Printf("scheduler: %s\n", result.SchedulerMode)
	if result.MatrixResult != "" {
		fmt.Printf("total_candidates: %d\n", result.TotalCandidates)
		if result.OriginalCandidates > 0 && result.OriginalCandidates != result.TotalCandidates {
			fmt.Printf("original_candidates: %d\n", result.OriginalCandidates)
		}
		if result.FeedbackFrom != "" {
			fmt.Printf("feedback_from: %s\n", result.FeedbackFrom)
		}
		if result.CandidateLimit > 0 {
			fmt.Printf("candidate_limit: %d\n", result.CandidateLimit)
		}
		if len(result.CandidateSummaries) > 0 {
			top := result.CandidateSummaries[0]
			fmt.Printf("top_candidate: %s score=%d cost=%d status=%s repro=%.2f%% avg_duration_ms=%d avg_artifact_bytes=%d\n",
				top.CandidateID,
				top.Score,
				top.CostPenalty,
				top.Status,
				top.ReproducibilityRate*100,
				top.AvgDurationMillis,
				top.AvgArtifactBytes,
			)
		}
		fmt.Printf("matrix_result: %s\n", result.MatrixResult)
	}
	fmt.Printf("differential: %t\n", result.Differential)
	fmt.Printf("total_runs: %d\n", result.TotalRuns)
	fmt.Printf("confirmed: %d\n", result.Confirmed)
	fmt.Printf("unconfirmed: %d\n", result.Unconfirmed)
	fmt.Printf("errors: %d\n", result.Errors)
	fmt.Printf("unique_signatures: %d\n", result.UniqueSignatures)
	fmt.Printf("discoveries: %d\n", len(result.Discoveries))
	fmt.Printf("corpus_entries: %d\n", len(result.CorpusEntries))
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func splitCases(value string) []string {
	return splitCSV(value)
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func campaign(args []string) {
	fs := flag.NewFlagSet("campaign", flag.ExitOnError)
	outDir := fs.String("out", "runs", "directory for campaign artifacts")
	corpusDir := fs.String("corpus", "corpus", "directory for interesting discovery corpus; empty disables corpus output")
	rounds := fs.Int("rounds", 2, "number of matrix feedback rounds")
	repeat := fs.Int("repeat", 1, "number of times to run each selected candidate per round")
	candidateLimit := fs.Int("candidate-limit", 0, "candidate budget for feedback-ranked rounds; 0 means all")
	caseList := fs.String("cases", "", "comma-separated testcase names; defaults to all")
	timingList := fs.String("timing", "", "comma-separated timing profile ids; defaults to all")
	feedbackFrom := fs.String("feedback-from", "", "optional seed matrix-result.json for the first round")
	delay := fs.Duration("delay", 1500*time.Millisecond, "delay passed through to testcase runs")
	mockURL := fs.String("mock-url", "", "optional EffectServer/AuthorityServer base URL")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	differential := fs.Bool("differential", false, "run selected candidates as control/fault pairs")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := scheduler.RunCampaign(context.Background(), scheduler.CampaignOptions{
		OutDir:           *outDir,
		CorpusDir:        *corpusDir,
		Rounds:           *rounds,
		Repeat:           *repeat,
		CandidateLimit:   *candidateLimit,
		Cases:            splitCases(*caseList),
		TimingProfileIDs: splitCSV(*timingList),
		Delay:            *delay,
		MockURL:          *mockURL,
		EnvKind:          *envKind,
		ContainerImage:   *containerImage,
		Differential:     *differential,
		FeedbackFrom:     *feedbackFrom,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz campaign failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("campaign_id: %s\n", result.CampaignID)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	fmt.Printf("rounds: %d\n", result.Rounds)
	fmt.Printf("candidate_limit: %d\n", result.CandidateLimit)
	fmt.Printf("total_suites: %d\n", result.TotalSuites)
	fmt.Printf("total_runs: %d\n", result.TotalRuns)
	fmt.Printf("confirmed: %d\n", result.Confirmed)
	fmt.Printf("unconfirmed: %d\n", result.Unconfirmed)
	fmt.Printf("errors: %d\n", result.Errors)
	fmt.Printf("discoveries: %d\n", result.Discoveries)
	fmt.Printf("corpus_entries: %d\n", result.CorpusEntries)
	fmt.Printf("unique_candidates: %d\n", result.UniqueCandidates)
	fmt.Printf("repeated_candidates: %d\n", result.RepeatedCandidates)
	for _, round := range result.RoundResults {
		fmt.Printf("round_%d: scheduler=%s candidates=%d runs=%d confirmed=%d errors=%d matrix_result=%s\n",
			round.Round,
			round.SchedulerMode,
			round.TotalCandidates,
			round.TotalRuns,
			round.Confirmed,
			round.Errors,
			round.MatrixResult,
		)
	}
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func runTarget(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "missing target subcommand: list, tasks, seeds, scenarios, groups, prompt-profiles, prompt-variants, footprint, plan-probes, matrix, minimize, run, suite, or campaign")
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		targetList()
	case "tasks":
		targetTasks()
	case "seeds":
		targetSeeds()
	case "scenarios":
		targetScenarios()
	case "groups":
		targetGroups()
	case "prompt-profiles":
		targetPromptProfiles()
	case "prompt-variants":
		targetPromptVariants()
	case "footprint":
		targetFootprint(args[1:])
	case "plan-probes":
		targetPlanProbes(args[1:])
	case "matrix":
		targetMatrix(args[1:])
	case "minimize":
		targetMinimize(args[1:])
	case "run":
		targetRun(args[1:])
	case "suite":
		targetSuite(args[1:])
	case "campaign":
		targetCampaign(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown target subcommand: %s\n", args[0])
		os.Exit(2)
	}
}

func targetList() {
	fmt.Printf("%-14s %-7s %-48s %s\n", "adapter", "ready", "capabilities", "description")
	for _, adapter := range target.TargetAdapters() {
		fmt.Printf("%-14s %-7t %-48s %s\n",
			adapter.AdapterID,
			adapter.Implemented,
			strings.Join(adapter.Capabilities, ","),
			adapter.Description,
		)
	}
}

func targetTasks() {
	fmt.Printf("%-28s %-26s %-24s %-5s %-20s %s\n", "task", "seed", "primitive", "late", "default_expected", "description")
	for _, task := range target.TargetTasks() {
		fmt.Printf("%-28s %-26s %-24s %-5t %-20s %s\n",
			task.TaskID,
			task.SeedID,
			task.PlantPrimitiveID,
			task.UsesLateObservation,
			strings.Join(task.DefaultExpectedFiles, ","),
			task.Description,
		)
	}
}

func targetSeeds() {
	fmt.Printf("%-28s %-26s %-22s %-24s %s\n", "seed", "primitives", "lifecycle_ops", "activation_kinds", "tasks")
	for _, seed := range target.TargetScenarioSeeds() {
		fmt.Printf("%-28s %-26s %-22s %-24s %s\n",
			seed.SeedID,
			strings.Join(seed.PlantPrimitives, ","),
			strings.Join(seed.LifecycleOperations, ","),
			strings.Join(seed.ActivationKinds, ","),
			strings.Join(seed.Tasks, ","),
		)
	}
}

func targetScenarios() {
	fmt.Printf("%-28s %-24s %-22s %-20s %-24s %s\n", "scenario", "seed", "primitive", "lifecycle_op", "activation", "mutations")
	for _, scenario := range target.TargetScenarios() {
		mutations := make([]string, 0, len(scenario.Mutations))
		for _, mutation := range scenario.Mutations {
			mutations = append(mutations, mutation.MutationID)
		}
		lifecycleOp := ""
		if scenario.ExecutionPlan != nil {
			lifecycleOp = scenario.ExecutionPlan.LifecycleOperationID
		}
		fmt.Printf("%-28s %-24s %-22s %-20s %-24s %s\n",
			scenario.ScenarioID,
			scenario.SeedID,
			scenario.PlantPrimitiveID,
			lifecycleOp,
			scenario.ActivationKindID,
			strings.Join(mutations, ","),
		)
	}
}

func targetGroups() {
	fmt.Printf("%-22s %-60s %s\n", "group", "tasks", "description")
	for _, group := range target.TargetTaskGroups() {
		fmt.Printf("%-22s %-60s %s\n",
			group.GroupID,
			strings.Join(group.Tasks, ","),
			group.Description,
		)
	}
}

func targetPromptProfiles() {
	fmt.Printf("%-12s %s\n", "profile", "description")
	for _, profile := range target.TargetPromptProfiles() {
		fmt.Printf("%-12s %s\n", profile.ProfileID, profile.Description)
	}
}

func targetPromptVariants() {
	fmt.Printf("%-18s %s\n", "variant", "description")
	for _, variant := range target.TargetPromptVariants() {
		fmt.Printf("%-18s %s\n", variant.VariantID, variant.Description)
	}
}

func targetFootprint(args []string) {
	fs := flag.NewFlagSet("target footprint", flag.ExitOnError)
	runDir := fs.String("run", "", "completed target run artifact directory")
	outPath := fs.String("out", "", "resource-footprint.json output path; defaults inside --run")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if strings.TrimSpace(*runDir) == "" {
		fmt.Fprintln(os.Stderr, "target footprint requires --run")
		os.Exit(2)
	}
	footprint, err := observation.ExtractTargetRunFootprint(*runDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target footprint failed: %v\n", err)
		os.Exit(1)
	}
	output := strings.TrimSpace(*outPath)
	if output == "" {
		output = filepath.Join(*runDir, observation.ResourceFootprintArtifact)
	}
	if err := observation.WriteFootprint(output, footprint); err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target footprint failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("query_id: %s\n", footprint.QueryID)
	if footprint.ScenarioID != "" {
		fmt.Printf("scenario_id: %s\n", footprint.ScenarioID)
	}
	fmt.Printf("resource_classes: %s\n", resourceClassCSV(footprint.ResourceClasses))
	fmt.Printf("paths: %d\n", len(footprint.Paths))
	fmt.Printf("processes: %d\n", len(footprint.Processes))
	fmt.Printf("artifact: %s\n", output)
}

func targetPlanProbes(args []string) {
	fs := flag.NewFlagSet("target plan-probes", flag.ExitOnError)
	footprintPath := fs.String("footprint", "", "resource-footprint.json input path")
	outPath := fs.String("out", "", "observation-plan.json output path; defaults beside --footprint")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if strings.TrimSpace(*footprintPath) == "" {
		fmt.Fprintln(os.Stderr, "target plan-probes requires --footprint")
		os.Exit(2)
	}
	footprint, err := observation.ReadFootprint(*footprintPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target plan-probes failed: %v\n", err)
		os.Exit(1)
	}
	plan, err := observation.CompilePlan(footprint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target plan-probes failed: %v\n", err)
		os.Exit(1)
	}
	output := strings.TrimSpace(*outPath)
	if output == "" {
		output = filepath.Join(filepath.Dir(*footprintPath), observation.ObservationPlanArtifact)
	}
	if err := observation.WritePlan(output, plan); err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target plan-probes failed: %v\n", err)
		os.Exit(1)
	}
	families := make([]string, 0, len(plan.ProbePlans))
	for _, probe := range plan.ProbePlans {
		families = append(families, string(probe.Family))
	}
	fmt.Printf("query_id: %s\n", plan.QueryID)
	fmt.Printf("probe_families: %s\n", strings.Join(families, ","))
	fmt.Printf("fallback_full_probe: %t\n", plan.FallbackFullProbe)
	fmt.Printf("artifact: %s\n", output)
}

func resourceClassCSV(classes []observation.ResourceClass) string {
	values := make([]string, 0, len(classes))
	for _, class := range classes {
		values = append(values, string(class))
	}
	return strings.Join(values, ",")
}

func targetMatrix(args []string) {
	fs := flag.NewFlagSet("target matrix", flag.ExitOnError)
	targetID := fs.String("target", "command", "human-readable target runtime id")
	taskID := fs.String("task", "orphan-process", "single target task id")
	taskList := fs.String("tasks", "", "comma-separated target task ids; overrides --task when set")
	seedID := fs.String("seed", "", "single built-in target scenario seed to expand into matrix candidates")
	seedList := fs.String("seeds", "", "comma-separated built-in target scenario seeds to expand before explicit tasks")
	taskGroup := fs.String("group", "", "single built-in target task group to expand into matrix candidates")
	taskGroups := fs.String("groups", "", "comma-separated built-in target task groups to expand before explicit tasks")
	promptProfile := fs.String("prompt-profile", "", "single built-in target prompt profile")
	promptProfiles := fs.String("prompt-profiles", "", "comma-separated built-in target prompt profiles; use all for every built-in profile")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	tasks, groups, seeds := resolveTargetTaskSelection(*taskID, *taskList, *seedID, *seedList, *taskGroup, *taskGroups, true)
	profileIDs := splitCSV(*promptProfiles)
	if len(profileIDs) == 0 && *promptProfile != "" {
		profileIDs = []string{*promptProfile}
	}
	result, err := scheduler.BuildTargetScheduleMatrix(scheduler.TargetMatrixOptions{
		TargetID:         *targetID,
		Tasks:            tasks,
		TaskGroups:       groups,
		SeedIDs:          seeds,
		PromptProfileIDs: profileIDs,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target matrix failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("schema: %s\n", result.SchemaVersion)
	fmt.Printf("target: %s\n", result.TargetID)
	fmt.Printf("tasks: %s\n", strings.Join(result.Tasks, ","))
	if len(result.TaskGroups) > 0 {
		fmt.Printf("task_groups: %s\n", strings.Join(result.TaskGroups, ","))
	}
	if len(result.SeedIDs) > 0 {
		fmt.Printf("seed_ids: %s\n", strings.Join(result.SeedIDs, ","))
	}
	if len(result.PromptProfiles) > 0 {
		fmt.Printf("prompt_profiles: %s\n", strings.Join(result.PromptProfiles, ","))
	}
	fmt.Printf("total_candidates: %d\n", result.TotalCandidates)
	fmt.Printf("%-52s %-24s %-22s %-20s %-10s %-18s %-5s %s\n", "candidate_id", "seed", "primitive", "lifecycle_op", "prompt", "variant", "late", "description")
	for _, candidate := range result.Candidates {
		fmt.Printf("%-52s %-24s %-22s %-20s %-10s %-18s %-5t %s\n",
			candidate.CandidateID,
			candidate.SeedID,
			candidate.PlantPrimitiveID,
			candidate.LifecycleOperationID,
			candidate.PromptProfileID,
			target.NormalizeTargetPromptVariantID(candidate.PromptVariantID),
			candidate.UsesLateObservation,
			candidate.Description,
		)
	}
}

func targetMinimize(args []string) {
	fs := flag.NewFlagSet("target minimize", flag.ExitOnError)
	sourcePath := fs.String("from", "", "target-suite-result.json or target-matrix-result.json to turn into a minimization batch")
	outDir := fs.String("out", "runs", "directory for target minimization artifacts")
	execute := fs.Bool("execute", false, "execute conservative prompt, command, Scenario IR, and execution-plan trials while preserving the source oracle constraints")
	candidateLimit := fs.Int("candidate-limit", 1, "maximum applicable candidates to execute when --execute is set; 0 means all")
	maxTrials := fs.Int("max-trials", 32, "maximum minimization trials per candidate when --execute is set")
	fidelity := fs.String("fidelity", string(scheduler.TargetMinimizationFidelityExact), "minimization fidelity mode: exact, semantic, or impact")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *execute {
		result, err := scheduler.RunTargetMinimization(context.Background(), scheduler.TargetMinimizationRunOptions{
			SourcePath:     *sourcePath,
			OutDir:         *outDir,
			CandidateLimit: *candidateLimit,
			MaxTrials:      *maxTrials,
			Fidelity:       scheduler.TargetMinimizationFidelity(*fidelity),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "syncfuzz target minimize failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("minimization_id: %s\n", result.MinimizationID)
		fmt.Printf("source_schema: %s\n", result.SourceSchema)
		fmt.Printf("fidelity: %s\n", result.Fidelity)
		fmt.Printf("applicable_plans: %d\n", result.ApplicablePlans)
		fmt.Printf("executed_candidates: %d\n", result.ExecutedCandidates)
		fmt.Printf("total_trials: %d\n", result.TotalTrials)
		fmt.Printf("accepted_reductions: %d\n", result.AcceptedReductions)
		for index, candidate := range result.Candidates {
			fmt.Printf("candidate_%d: task=%s preserved=%t prompt_lines=%d->%d command_lines=%d->%d components=%d->%d mutations=%d->%d trials=%d accepted=%d prompt_accepted=%d command_accepted=%d component_accepted=%d mutation_accepted=%d activation_accepted=%d execution_accepted=%d",
				index+1,
				candidate.TaskID,
				candidate.Preserved,
				candidate.OriginalPromptLines,
				candidate.MinimizedPromptLines,
				candidate.OriginalCommandLines,
				candidate.MinimizedCommandLines,
				candidate.OriginalComponents,
				candidate.MinimizedComponents,
				candidate.OriginalMutations,
				candidate.MinimizedMutations,
				candidate.Trials,
				candidate.AcceptedReductions,
				candidate.AcceptedPromptReductions,
				candidate.AcceptedCommandReductions,
				candidate.AcceptedComponentReductions,
				candidate.AcceptedMutationReductions,
				candidate.AcceptedActivationReductions,
				candidate.AcceptedExecutionReductions,
			)
			if len(candidate.AcceptedSteps) > 0 {
				fmt.Printf(" steps=%s", strings.Join(candidate.AcceptedSteps, ","))
			}
			if candidate.Error != "" {
				fmt.Printf(" error=%q", candidate.Error)
			}
			fmt.Println()
		}
		fmt.Printf("artifacts: %s\n", result.ArtifactDir)
		return
	}

	result, err := scheduler.BuildTargetMinimizationBatch(scheduler.TargetMinimizationBatchOptions{
		SourcePath: *sourcePath,
		OutDir:     *outDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target minimize failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("batch_id: %s\n", result.BatchID)
	fmt.Printf("source_schema: %s\n", result.SourceSchemaVersion)
	fmt.Printf("total_results: %d\n", result.TotalResults)
	fmt.Printf("applicable_plans: %d\n", result.ApplicablePlans)
	fmt.Printf("skipped_plans: %d\n", result.SkippedPlans)
	if len(result.Plans) > 0 {
		first := result.Plans[0]
		fmt.Printf("first_plan: task=%s run=%s applicable=%t steps=%d\n",
			first.TaskID,
			first.RunID,
			first.MinimizationPlan.Applicable,
			len(first.MinimizationPlan.Steps),
		)
	}
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func targetRun(args []string) {
	fs := flag.NewFlagSet("target run", flag.ExitOnError)
	adapterID := fs.String("adapter", "command", "target adapter id")
	targetID := fs.String("target", "command", "human-readable target runtime id")
	taskID := fs.String("task", "orphan-process", "target task id")
	objective := fs.String("objective", "", "optional target objective")
	promptProfile := fs.String("prompt-profile", "", "built-in target prompt profile used when no explicit prompt text or prompt file is provided")
	prompt := fs.String("prompt", "", "inline prompt passed through SYNCFUZZ_PROMPT")
	promptFile := fs.String("prompt-file", "", "optional prompt file")
	command := fs.String("command", "", "target command to run inside the SyncFuzz workspace")
	commandFile := fs.String("command-file", "", "optional file containing the target command")
	expectFiles := fs.String("expect-files", "", "comma-separated files expected to exist after the target run")
	outDir := fs.String("out", "runs", "directory for target run artifacts")
	workspace := fs.String("workspace", "", "optional workspace; defaults to runs/<run_id>/workspace")
	timeout := fs.Duration("timeout", 2*time.Minute, "target command timeout")
	observeDelay := fs.Duration("observe-delay", 0, "delay after target command return before final observation; 0 uses the adapter default")
	lateObserveDelay := fs.Duration("late-observe-delay", 0, "optional delay after immediate observation for delayed target effects")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := target.RunTarget(context.Background(), target.TargetRunOptions{
		AdapterID:        *adapterID,
		TargetID:         *targetID,
		TaskID:           *taskID,
		Objective:        *objective,
		PromptProfileID:  *promptProfile,
		Prompt:           *prompt,
		PromptFile:       *promptFile,
		Command:          *command,
		CommandFile:      *commandFile,
		OutDir:           *outDir,
		Workspace:        *workspace,
		Timeout:          *timeout,
		ObserveDelay:     *observeDelay,
		LateObserveDelay: *lateObserveDelay,
		EnvKind:          *envKind,
		ContainerImage:   *containerImage,
		ExpectedFiles:    splitCSV(*expectFiles),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target run failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("run_id: %s\n", result.RunID)
	fmt.Printf("adapter: %s\n", result.AdapterID)
	fmt.Printf("target: %s\n", result.TargetID)
	fmt.Printf("task: %s\n", result.TaskID)
	if result.PromptProfileID != "" {
		fmt.Printf("prompt_profile: %s\n", result.PromptProfileID)
	}
	if result.PromptVariantID != "" {
		fmt.Printf("prompt_variant: %s\n", result.PromptVariantID)
	}
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	fmt.Printf("completed: %t\n", result.Completed)
	fmt.Printf("expectations_met: %t\n", result.ExpectationsMet)
	fmt.Printf("target_oracle: %s\n", result.TargetOracle.Name)
	if result.TargetOracle.Status != "" {
		fmt.Printf("oracle_status: %s\n", result.TargetOracle.Status)
	}
	fmt.Printf("oracle_confirmed: %t\n", result.TargetOracle.Confirmed)
	if result.TaskCompliance.Name != "" {
		fmt.Printf("task_compliance: %s\n", result.TaskCompliance.Name)
	}
	if result.TaskCompliance.Status != "" {
		fmt.Printf("task_compliance_status: %s\n", result.TaskCompliance.Status)
	}
	if result.ContractInterpretation != nil {
		if result.ContractInterpretation.Status != "" {
			fmt.Printf("contract_status: %s\n", result.ContractInterpretation.Status)
		}
		if result.ContractInterpretation.RuleID != "" {
			fmt.Printf("contract_rule: %s\n", result.ContractInterpretation.RuleID)
		}
	}
	if result.TargetOracle.Attribution != "" {
		fmt.Printf("oracle_attribution: %s\n", result.TargetOracle.Attribution)
	}
	if len(result.ExpectedFilesPresent) > 0 {
		fmt.Printf("expected_present: %s\n", strings.Join(result.ExpectedFilesPresent, ","))
	}
	if len(result.ExpectedFilesMissing) > 0 {
		fmt.Printf("expected_missing: %s\n", strings.Join(result.ExpectedFilesMissing, ","))
	}
	if result.LateObserved {
		fmt.Printf("late_observe_delay_ms: %d\n", result.LateObserveDelayMs)
		if len(result.LateExpectedFilesPresent) > 0 {
			fmt.Printf("late_expected_present: %s\n", strings.Join(result.LateExpectedFilesPresent, ","))
		}
		if len(result.LateExpectedFilesMissing) > 0 {
			fmt.Printf("late_expected_missing: %s\n", strings.Join(result.LateExpectedFilesMissing, ","))
		}
	}
	fmt.Printf("exit_code: %d\n", result.CommandResult.ExitCode)
	fmt.Printf("timed_out: %t\n", result.CommandResult.TimedOut)
	fmt.Printf("duration_ms: %d\n", result.CommandResult.DurationMs)
	fmt.Printf("observe_delay_ms: %d\n", result.ObserveDelayMs)
	fmt.Printf("output_bytes: %d\n", result.CommandResult.OutputBytes)
	fmt.Printf("workspace: %s\n", result.Workspace)
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func targetSuite(args []string) {
	fs := flag.NewFlagSet("target suite", flag.ExitOnError)
	adapterID := fs.String("adapter", "command", "target adapter id")
	targetID := fs.String("target", "command", "human-readable target runtime id")
	taskID := fs.String("task", "orphan-process", "single target task id")
	taskList := fs.String("tasks", "", "comma-separated target task ids; overrides --task when set")
	seedID := fs.String("seed", "", "single built-in target scenario seed to expand into suite tasks")
	seedList := fs.String("seeds", "", "comma-separated built-in target scenario seeds to expand before explicit tasks")
	taskGroup := fs.String("group", "", "single built-in target task group to expand into suite tasks")
	taskGroups := fs.String("groups", "", "comma-separated built-in target task groups to expand before explicit tasks")
	objective := fs.String("objective", "", "optional shared objective override")
	promptProfile := fs.String("prompt-profile", "", "single built-in target prompt profile")
	promptProfiles := fs.String("prompt-profiles", "", "comma-separated built-in target prompt profiles; use all for every built-in profile")
	prompt := fs.String("prompt", "", "inline prompt passed through SYNCFUZZ_PROMPT")
	promptFile := fs.String("prompt-file", "", "optional shared prompt file")
	command := fs.String("command", "", "target command to run inside the SyncFuzz workspace")
	commandFile := fs.String("command-file", "", "optional file containing the target command")
	expectFiles := fs.String("expect-files", "", "comma-separated files expected to exist after every target task")
	outDir := fs.String("out", "runs", "directory for target suite artifacts")
	corpusDir := fs.String("corpus", "corpus", "directory for confirmed target corpus entries; empty disables corpus output")
	repeat := fs.Int("repeat", 1, "number of repetitions per target task")
	timeout := fs.Duration("timeout", 2*time.Minute, "target command timeout")
	observeDelay := fs.Duration("observe-delay", 0, "delay after target command return before final observation; 0 uses the adapter default")
	lateObserveDelay := fs.Duration("late-observe-delay", 0, "optional delay after immediate observation for delayed target effects")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	matrixMode := fs.Bool("matrix", false, "run the real-target task matrix instead of a fixed task list")
	feedbackFrom := fs.String("feedback-from", "", "previous target-matrix-result.json used to rank target candidates")
	candidateLimit := fs.Int("candidate-limit", 0, "maximum matrix candidates to execute after feedback ranking; 0 means all")
	selectionPolicy := fs.String("selection-policy", "", "matrix candidate selection policy: explore, feedback, fixed, or random")
	randomSeed := fs.Int64("random-seed", scheduler.DefaultTargetRandomSeed, "deterministic seed used by --selection-policy random")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	tasks, groups, seeds := resolveTargetTaskSelection(*taskID, *taskList, *seedID, *seedList, *taskGroup, *taskGroups, *matrixMode)
	profileIDs := splitCSV(*promptProfiles)
	if len(profileIDs) == 0 && *promptProfile != "" {
		profileIDs = []string{*promptProfile}
	}

	result, err := scheduler.RunTargetSuite(context.Background(), scheduler.TargetSuiteOptions{
		AdapterID:        *adapterID,
		TargetID:         *targetID,
		Tasks:            tasks,
		TaskGroups:       groups,
		SeedIDs:          seeds,
		Objective:        *objective,
		PromptProfileID:  *promptProfile,
		PromptProfileIDs: profileIDs,
		Prompt:           *prompt,
		PromptFile:       *promptFile,
		Command:          *command,
		CommandFile:      *commandFile,
		OutDir:           *outDir,
		CorpusDir:        *corpusDir,
		Repeat:           *repeat,
		Timeout:          *timeout,
		ObserveDelay:     *observeDelay,
		LateObserveDelay: *lateObserveDelay,
		EnvKind:          *envKind,
		ContainerImage:   *containerImage,
		ExpectedFiles:    splitCSV(*expectFiles),
		Matrix:           *matrixMode,
		FeedbackFrom:     *feedbackFrom,
		CandidateLimit:   *candidateLimit,
		SelectionPolicy:  scheduler.TargetSelectionPolicy(*selectionPolicy),
		RandomSeed:       *randomSeed,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target suite failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("suite_id: %s\n", result.SuiteID)
	fmt.Printf("adapter: %s\n", result.AdapterID)
	fmt.Printf("target: %s\n", result.TargetID)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	fmt.Printf("repeat: %d\n", result.Repeat)
	fmt.Printf("tasks: %s\n", strings.Join(result.Tasks, ","))
	if len(result.TaskGroups) > 0 {
		fmt.Printf("task_groups: %s\n", strings.Join(result.TaskGroups, ","))
	}
	if len(result.SeedIDs) > 0 {
		fmt.Printf("seed_ids: %s\n", strings.Join(result.SeedIDs, ","))
	}
	if len(result.PromptProfiles) > 0 {
		fmt.Printf("prompt_profiles: %s\n", strings.Join(result.PromptProfiles, ","))
	}
	if result.MatrixResult != "" {
		fmt.Printf("scheduler: %s\n", result.SchedulerMode)
		fmt.Printf("total_candidates: %d\n", result.TotalCandidates)
		if result.OriginalCandidates > 0 && result.OriginalCandidates != result.TotalCandidates {
			fmt.Printf("original_candidates: %d\n", result.OriginalCandidates)
		}
		if result.FeedbackFrom != "" {
			fmt.Printf("feedback_from: %s\n", result.FeedbackFrom)
		}
		if result.CandidateLimit > 0 {
			fmt.Printf("candidate_limit: %d\n", result.CandidateLimit)
		}
		if result.SelectionPolicy != "" {
			fmt.Printf("selection_policy: %s\n", result.SelectionPolicy)
		}
		if result.RandomSeed != 0 {
			fmt.Printf("random_seed: %d\n", result.RandomSeed)
		}
		if len(result.CandidateSummaries) > 0 {
			top := result.CandidateSummaries[0]
			fmt.Printf("top_candidate: %s score=%d status=%s repro=%.2f%% avg_duration_ms=%d avg_artifact_bytes=%d\n",
				top.CandidateID,
				top.Score,
				top.Status,
				top.ReproducibilityRate*100,
				top.AvgDurationMillis,
				top.AvgArtifactBytes,
			)
		}
		if len(result.FrontierCandidates) > 0 {
			next := result.FrontierCandidates[0]
			fmt.Printf("next_candidate: %s mode=%s gap_score=%d novelty_score=%d\n",
				next.CandidateID,
				next.SelectionMode,
				next.GapScore,
				next.NoveltyScore,
			)
		}
		fmt.Printf("matrix_result: %s\n", result.MatrixResult)
	}
	fmt.Printf("total_runs: %d\n", result.TotalRuns)
	fmt.Printf("confirmed: %d\n", result.Confirmed)
	fmt.Printf("unconfirmed: %d\n", result.Unconfirmed)
	fmt.Printf("errors: %d\n", result.Errors)
	for _, stats := range result.AttributionSummaries {
		fmt.Printf("attribution[%s]: total=%d confirmed=%d unconfirmed=%d\n",
			stats.Attribution,
			stats.TotalRuns,
			stats.Confirmed,
			stats.Unconfirmed,
		)
	}
	for _, stats := range result.ComplianceSummaries {
		fmt.Printf("compliance[%s]: total=%d confirmed=%d unconfirmed=%d\n",
			stats.Status,
			stats.TotalRuns,
			stats.Confirmed,
			stats.Unconfirmed,
		)
	}
	for _, stats := range result.ContractSummaries {
		fmt.Printf("contract[%s]: total=%d confirmed=%d unconfirmed=%d\n",
			stats.Status,
			stats.TotalRuns,
			stats.Confirmed,
			stats.Unconfirmed,
		)
	}
	fmt.Printf("corpus_entries: %d\n", len(result.CorpusEntries))
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func targetCampaign(args []string) {
	fs := flag.NewFlagSet("target campaign", flag.ExitOnError)
	adapterID := fs.String("adapter", "command", "target adapter id")
	targetID := fs.String("target", "command", "human-readable target runtime id")
	taskID := fs.String("task", "orphan-process", "single target task id")
	taskList := fs.String("tasks", "", "comma-separated target task ids; overrides --task when set")
	seedID := fs.String("seed", "", "single built-in target scenario seed to expand into campaign candidates")
	seedList := fs.String("seeds", "", "comma-separated built-in target scenario seeds to expand before explicit tasks")
	taskGroup := fs.String("group", "", "single built-in target task group to expand into campaign candidates")
	taskGroups := fs.String("groups", "", "comma-separated built-in target task groups to expand before explicit tasks")
	objective := fs.String("objective", "", "optional shared objective override")
	promptProfile := fs.String("prompt-profile", "", "single built-in target prompt profile")
	promptProfiles := fs.String("prompt-profiles", "", "comma-separated built-in target prompt profiles; use all for every built-in profile")
	prompt := fs.String("prompt", "", "inline prompt passed through SYNCFUZZ_PROMPT")
	promptFile := fs.String("prompt-file", "", "optional shared prompt file")
	command := fs.String("command", "", "target command to run inside the SyncFuzz workspace")
	commandFile := fs.String("command-file", "", "optional file containing the target command")
	expectFiles := fs.String("expect-files", "", "comma-separated files expected to exist after every target task")
	outDir := fs.String("out", "runs", "directory for target campaign artifacts")
	corpusDir := fs.String("corpus", "corpus", "directory for confirmed target corpus entries; empty disables corpus output")
	rounds := fs.Int("rounds", 2, "number of target feedback rounds")
	repeat := fs.Int("repeat", 1, "number of repetitions per target candidate")
	candidateLimit := fs.Int("candidate-limit", 0, "candidate budget for feedback-ranked rounds; 0 means all")
	feedbackFrom := fs.String("feedback-from", "", "optional seed target-matrix-result.json for the first round")
	selectionPolicy := fs.String("selection-policy", "", "matrix candidate selection policy: explore, feedback, fixed, or random")
	randomSeed := fs.Int64("random-seed", scheduler.DefaultTargetRandomSeed, "deterministic seed used by --selection-policy random")
	minCoverageGainScore := fs.Int("min-coverage-gain-score", 0, "minimum round coverage gain weighted score before a round counts as stagnant")
	maxStagnantRounds := fs.Int("max-stagnant-rounds", 0, "stop early after this many consecutive stagnant rounds; 0 disables early stop")
	autoPivot := fs.Bool("auto-pivot", false, "when stagnation is detected, expand into a recommended missing dimension instead of stopping early")
	timeout := fs.Duration("timeout", 2*time.Minute, "target command timeout")
	observeDelay := fs.Duration("observe-delay", 0, "delay after target command return before final observation; 0 uses the adapter default")
	lateObserveDelay := fs.Duration("late-observe-delay", 0, "optional delay after immediate observation for delayed target effects")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	tasks, groups, seeds := resolveTargetTaskSelection(*taskID, *taskList, *seedID, *seedList, *taskGroup, *taskGroups, true)
	profileIDs := splitCSV(*promptProfiles)
	if len(profileIDs) == 0 && *promptProfile != "" {
		profileIDs = []string{*promptProfile}
	}
	result, err := scheduler.RunTargetCampaign(context.Background(), scheduler.TargetCampaignOptions{
		AdapterID:            *adapterID,
		TargetID:             *targetID,
		Tasks:                tasks,
		TaskGroups:           groups,
		SeedIDs:              seeds,
		Objective:            *objective,
		PromptProfileID:      *promptProfile,
		PromptProfileIDs:     profileIDs,
		Prompt:               *prompt,
		PromptFile:           *promptFile,
		Command:              *command,
		CommandFile:          *commandFile,
		OutDir:               *outDir,
		CorpusDir:            *corpusDir,
		Rounds:               *rounds,
		Repeat:               *repeat,
		CandidateLimit:       *candidateLimit,
		FeedbackFrom:         *feedbackFrom,
		SelectionPolicy:      scheduler.TargetSelectionPolicy(*selectionPolicy),
		RandomSeed:           *randomSeed,
		MinCoverageGainScore: *minCoverageGainScore,
		MaxStagnantRounds:    *maxStagnantRounds,
		AutoPivot:            *autoPivot,
		Timeout:              *timeout,
		ObserveDelay:         *observeDelay,
		LateObserveDelay:     *lateObserveDelay,
		EnvKind:              *envKind,
		ContainerImage:       *containerImage,
		ExpectedFiles:        splitCSV(*expectFiles),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz target campaign failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("campaign_id: %s\n", result.CampaignID)
	fmt.Printf("adapter: %s\n", result.AdapterID)
	fmt.Printf("target: %s\n", result.TargetID)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	fmt.Printf("rounds: %d\n", result.Rounds)
	if len(result.PromptProfiles) > 0 {
		fmt.Printf("prompt_profiles: %s\n", strings.Join(result.PromptProfiles, ","))
	}
	if len(result.SeedIDs) > 0 {
		fmt.Printf("seed_ids: %s\n", strings.Join(result.SeedIDs, ","))
	}
	fmt.Printf("candidate_limit: %d\n", result.CandidateLimit)
	if result.SelectionPolicy != "" {
		fmt.Printf("selection_policy: %s\n", result.SelectionPolicy)
	}
	if result.RandomSeed != 0 {
		fmt.Printf("random_seed: %d\n", result.RandomSeed)
	}
	if result.MaxStagnantRounds > 0 {
		fmt.Printf("min_coverage_gain_score: %d\n", result.MinCoverageGainScore)
		fmt.Printf("max_stagnant_rounds: %d\n", result.MaxStagnantRounds)
		fmt.Printf("auto_pivot: %t\n", result.AutoPivot)
	}
	fmt.Printf("total_suites: %d\n", result.TotalSuites)
	fmt.Printf("total_runs: %d\n", result.TotalRuns)
	fmt.Printf("confirmed: %d\n", result.Confirmed)
	fmt.Printf("unconfirmed: %d\n", result.Unconfirmed)
	fmt.Printf("errors: %d\n", result.Errors)
	fmt.Printf("corpus_entries: %d\n", result.CorpusEntries)
	fmt.Printf("unique_candidates: %d\n", result.UniqueCandidates)
	fmt.Printf("repeated_candidates: %d\n", result.RepeatedCandidates)
	if result.StoppedEarly {
		fmt.Printf("stopped_early: true\n")
		fmt.Printf("stop_reason: %s\n", result.StopReason)
	}
	if result.CatalogExhausted {
		fmt.Printf("catalog_exhausted: true\n")
	}
	for _, pivot := range result.PivotHistory {
		fmt.Printf("pivot_round_%d: dimension=%s values=%s",
			pivot.AfterRound,
			pivot.Dimension,
			strings.Join(pivot.Values, ","),
		)
		if pivot.FrontierCandidate != "" {
			fmt.Printf(" frontier_candidate=%s gap_score=%d novelty_score=%d mode=%s",
				pivot.FrontierCandidate,
				pivot.FrontierGapScore,
				pivot.FrontierNovelty,
				pivot.FrontierSelection,
			)
		}
		if pivot.NewCandidateCount > 0 {
			fmt.Printf(" new_candidates=%d", pivot.NewCandidateCount)
		}
		fmt.Println()
	}
	for _, recommendation := range result.PivotRecommendations {
		fmt.Printf("pivot[%s]: %s", recommendation.Dimension, strings.Join(recommendation.Values, ","))
		if recommendation.Reason != "" {
			fmt.Printf(" reason=%s", recommendation.Reason)
		}
		fmt.Println()
	}
	for _, frontier := range result.FrontierCandidates {
		fmt.Printf("frontier_%d: candidate=%s mode=%s gap_score=%d novelty_score=%d\n",
			frontier.Rank,
			frontier.CandidateID,
			frontier.SelectionMode,
			frontier.GapScore,
			frontier.NoveltyScore,
		)
	}
	for _, round := range result.RoundResults {
		fmt.Printf("round_%d: scheduler=%s policy=%s candidates=%d runs=%d confirmed=%d errors=%d matrix_result=%s\n",
			round.Round,
			round.SchedulerMode,
			round.SelectionPolicy,
			round.TotalCandidates,
			round.TotalRuns,
			round.Confirmed,
			round.Errors,
			round.MatrixResult,
		)
	}
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func resolveTargetTaskSelection(taskID string, taskList string, seedID string, seedList string, taskGroup string, taskGroups string, matrixMode bool) ([]string, []string, []string) {
	groups := splitCSV(taskGroups)
	if taskGroup != "" {
		groups = append([]string{taskGroup}, groups...)
	}
	seeds := splitCSV(seedList)
	if seedID != "" {
		seeds = append([]string{seedID}, seeds...)
	}
	tasks := splitCSV(taskList)
	if len(tasks) == 0 && len(groups) == 0 && len(seeds) == 0 {
		if matrixMode {
			if taskID != "orphan-process" {
				tasks = []string{taskID}
			}
		} else {
			tasks = []string{taskID}
		}
	} else if len(tasks) == 0 && taskID != "orphan-process" {
		tasks = []string{taskID}
	}
	return tasks, groups, seeds
}

func runCorpus(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "missing corpus subcommand: list, show, or verify")
		os.Exit(2)
	}

	switch args[0] {
	case "list":
		corpusList(args[1:])
	case "analyze":
		corpusAnalyze(args[1:])
	case "show":
		corpusShow(args[1:])
	case "verify":
		corpusVerify(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown corpus subcommand: %s\n", args[0])
		os.Exit(2)
	}
}

func corpusAnalyze(args []string) {
	fs := flag.NewFlagSet("corpus analyze", flag.ExitOnError)
	corpusDir := fs.String("corpus", "corpus", "corpus directory")
	limit := fs.Int("limit", 0, "maximum entries to analyze; 0 means all")
	verificationFile := fs.String("verification", "", "optional verification-result.json to include replay outcome summaries")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := corpus.AnalyzeCorpus(corpus.CorpusAnalyzeOptions{
		CorpusDir:              *corpusDir,
		Limit:                  *limit,
		VerificationResultFile: *verificationFile,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz corpus analyze failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("corpus: %s\n", result.CorpusDir)
	fmt.Printf("total_entries: %d\n", result.TotalEntries)
	printCorpusAnalysisFieldStats("execution", result.ExecutionSummaries)
	printCorpusAnalysisFieldStats("target_oracle", result.TargetOracleSummaries)
	printCorpusAnalysisFieldStats("attribution", result.AttributionSummaries)
	printCorpusAnalysisFieldStats("task_compliance", result.TaskComplianceSummaries)
	printCorpusAnalysisFieldStats("contract", result.ContractSummaries)
	for _, subject := range result.SubjectSummaries {
		if subject.ExecutionKind == "target" {
			fmt.Printf("subject[target:%s/%s]: total=%d\n", subject.TargetID, subject.TaskID, subject.TotalEntries)
			printCorpusAnalysisFieldStats("  attribution", subject.AttributionSummaries)
			printCorpusAnalysisFieldStats("  contract", subject.ContractSummaries)
			continue
		}
		fmt.Printf("subject[case:%s]: total=%d\n", subject.CaseName, subject.TotalEntries)
	}
	if result.VerificationID != "" {
		fmt.Printf("verification_id: %s\n", result.VerificationID)
		for _, stats := range result.VerificationOutcomeSummaries {
			fmt.Printf("verify_outcome[%s]: total=%d\n", stats.Category, stats.TotalEntries)
		}
		for _, stats := range result.VerificationSubjectSummaries {
			if stats.ExecutionKind == "target" {
				fmt.Printf("verify_subject[target:%s/%s]: total=%d reproduced=%d signature_drift=%d unconfirmed=%d errors=%d\n",
					stats.TargetID,
					stats.TaskID,
					stats.TotalEntries,
					stats.Reproduced,
					stats.SignatureDrift,
					stats.Unconfirmed,
					stats.Errors,
				)
				continue
			}
			fmt.Printf("verify_subject[case:%s]: total=%d reproduced=%d signature_drift=%d unconfirmed=%d errors=%d\n",
				stats.CaseName,
				stats.TotalEntries,
				stats.Reproduced,
				stats.SignatureDrift,
				stats.Unconfirmed,
				stats.Errors,
			)
		}
	}
}

func printCorpusAnalysisFieldStats(prefix string, stats []corpus.CorpusAnalysisFieldStats) {
	for _, item := range stats {
		fmt.Printf("%s[%s]: total=%d\n", prefix, item.Key, item.TotalEntries)
	}
}

func corpusList(args []string) {
	fs := flag.NewFlagSet("corpus list", flag.ExitOnError)
	corpusDir := fs.String("corpus", "corpus", "corpus directory")
	limit := fs.Int("limit", 20, "maximum entries to print; 0 means all")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	entries, err := corpus.ListCorpus(*corpusDir, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz corpus list failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-48s %-18s %-8s %-36s %-5s %s\n", "entry_id", "kind", "exec", "subject", "score", "signature")
	for _, entry := range entries {
		fmt.Printf("%-48s %-18s %-8s %-36s %-5d %s\n",
			entry.EntryID,
			entry.Kind,
			entry.EffectiveExecutionKind(),
			entry.Subject(),
			entry.Score,
			entry.Signature.String(),
		)
	}
}

func corpusShow(args []string) {
	fs := flag.NewFlagSet("corpus show", flag.ExitOnError)
	corpusDir := fs.String("corpus", "corpus", "corpus directory")
	entryID := fs.String("id", "", "corpus entry id")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	entry, err := corpus.ShowCorpusEntry(*corpusDir, *entryID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz corpus show failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("entry_id: %s\n", entry.EntryID)
	fmt.Printf("execution_kind: %s\n", entry.EffectiveExecutionKind())
	fmt.Printf("kind: %s\n", entry.Kind)
	fmt.Printf("score: %d\n", entry.Score)
	if entry.EffectiveExecutionKind() == "target" {
		fmt.Printf("adapter: %s\n", entry.AdapterID)
		fmt.Printf("target: %s\n", entry.TargetID)
		fmt.Printf("task: %s\n", entry.TaskID)
		if entry.PromptProfileID != "" {
			fmt.Printf("prompt_profile: %s\n", entry.PromptProfileID)
		}
		if entry.PromptVariantID != "" {
			fmt.Printf("prompt_variant: %s\n", entry.PromptVariantID)
		}
		if entry.TargetOracleStatus != "" {
			fmt.Printf("target_oracle_status: %s\n", entry.TargetOracleStatus)
		}
		if entry.TargetAttribution != "" {
			fmt.Printf("target_attribution: %s\n", entry.TargetAttribution)
		}
		if entry.TaskComplianceStatus != "" {
			fmt.Printf("task_compliance_status: %s\n", entry.TaskComplianceStatus)
		}
		if entry.ContractStatus != "" {
			fmt.Printf("contract_status: %s\n", entry.ContractStatus)
		}
		fmt.Printf("subject: %s\n", entry.Subject())
	} else {
		fmt.Printf("case: %s\n", entry.CaseName)
	}
	if entry.CandidateID != "" {
		fmt.Printf("candidate: %s\n", entry.CandidateID)
	}
	if entry.PrimitiveID != "" {
		fmt.Printf("primitive: %s\n", entry.PrimitiveID)
	}
	fmt.Printf("suite_id: %s\n", entry.SuiteID)
	fmt.Printf("run_id: %s\n", entry.RunID)
	if entry.PairID != "" {
		fmt.Printf("pair_id: %s\n", entry.PairID)
		fmt.Printf("control_run_id: %s\n", entry.ControlRunID)
		fmt.Printf("fault_run_id: %s\n", entry.FaultRunID)
		fmt.Printf("differential: %t\n", entry.Differential)
		fmt.Printf("security_relevant: %t\n", entry.SecurityRelevant)
		fmt.Printf("differential_report: %s\n", entry.DifferentialReport)
	}
	if entry.FaultPlanID != "" {
		fmt.Printf("fault_plan: %s\n", entry.FaultPlanID)
	}
	printTimingProfile(entry.TimingProfileID)
	fmt.Printf("signature: %s\n", entry.Signature.String())
	fmt.Printf("artifact_dir: %s\n", entry.ArtifactDir)
	fmt.Printf("recorded_at: %s\n", entry.RecordedAt)
}

func corpusVerify(args []string) {
	fs := flag.NewFlagSet("corpus verify", flag.ExitOnError)
	corpusDir := fs.String("corpus", "corpus", "corpus directory")
	outDir := fs.String("out", "runs", "directory for verification artifacts")
	limit := fs.Int("limit", 0, "maximum entries to verify; 0 means all")
	delay := fs.Duration("delay", 1500*time.Millisecond, "delay passed through to testcase runs")
	mockURL := fs.String("mock-url", "", "optional EffectServer/AuthorityServer base URL")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	timingProfileID := fs.String("timing", "", "optional timing profile override")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := corpus.VerifyCorpus(context.Background(), corpus.VerifyOptions{
		CorpusDir:       *corpusDir,
		OutDir:          *outDir,
		Limit:           *limit,
		Delay:           *delay,
		MockURL:         *mockURL,
		EnvKind:         *envKind,
		ContainerImage:  *containerImage,
		TimingProfileID: *timingProfileID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz corpus verify failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("verification_id: %s\n", result.VerificationID)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	fmt.Printf("total_entries: %d\n", result.TotalEntries)
	fmt.Printf("verified: %d\n", result.Verified)
	fmt.Printf("reproduced: %d\n", result.Reproduced)
	fmt.Printf("failed: %d\n", result.Failed)
	fmt.Printf("signature_drift: %d\n", result.SignatureDrift)
	fmt.Printf("unconfirmed: %d\n", result.Unconfirmed)
	fmt.Printf("errors: %d\n", result.Errors)
	for _, stats := range result.OutcomeSummaries {
		fmt.Printf("outcome[%s]: total=%d\n", stats.Category, stats.TotalEntries)
	}
	for _, stats := range result.SubjectSummaries {
		if stats.ExecutionKind == "target" {
			fmt.Printf("subject[target:%s/%s]: total=%d reproduced=%d signature_drift=%d unconfirmed=%d errors=%d\n",
				stats.TargetID,
				stats.TaskID,
				stats.TotalEntries,
				stats.Reproduced,
				stats.SignatureDrift,
				stats.Unconfirmed,
				stats.Errors,
			)
		} else {
			fmt.Printf("subject[case:%s]: total=%d reproduced=%d signature_drift=%d unconfirmed=%d errors=%d\n",
				stats.CaseName,
				stats.TotalEntries,
				stats.Reproduced,
				stats.SignatureDrift,
				stats.Unconfirmed,
				stats.Errors,
			)
		}
	}
	fmt.Printf("reproducibility_rate: %.2f%%\n", result.ReproducibilityRate*100)
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
	if result.Failed > 0 {
		os.Exit(1)
	}
}

func replay(args []string) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	corpusDir := fs.String("corpus", "corpus", "corpus directory")
	entryID := fs.String("id", "", "corpus entry id or unique prefix")
	outDir := fs.String("out", "runs", "directory for replay artifacts")
	delay := fs.Duration("delay", 1500*time.Millisecond, "delay passed through to testcase run")
	mockURL := fs.String("mock-url", "", "optional EffectServer/AuthorityServer base URL")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	faultPlanID := fs.String("fault-plan", "", "optional replay fault plan override")
	primitiveID := fs.String("primitive", "", "optional replay primitive override")
	timingProfileID := fs.String("timing", "", "optional replay timing profile override")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := corpus.ReplayCorpusEntry(context.Background(), corpus.ReplayOptions{
		CorpusDir:       *corpusDir,
		EntryID:         *entryID,
		OutDir:          *outDir,
		Delay:           *delay,
		MockURL:         *mockURL,
		EnvKind:         *envKind,
		ContainerImage:  *containerImage,
		FaultPlanID:     *faultPlanID,
		PrimitiveID:     *primitiveID,
		TimingProfileID: *timingProfileID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz replay failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("replay_id: %s\n", result.ReplayID)
	fmt.Printf("entry_id: %s\n", result.EntryID)
	fmt.Printf("execution_kind: %s\n", result.ExecutionKind)
	if result.ExecutionKind == "target" {
		fmt.Printf("target: %s\n", result.TargetID)
		fmt.Printf("task: %s\n", result.TaskID)
		fmt.Printf("subject: %s\n", result.CaseName)
	} else {
		fmt.Printf("case: %s\n", result.CaseName)
	}
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	printFaultPlan(result.FaultPlanID)
	printPrimitive(result.PrimitiveID)
	printTimingProfile(result.TimingProfileID)
	fmt.Printf("confirmed: %t\n", result.Confirmed)
	fmt.Printf("signature_matched: %t\n", result.SignatureMatched)
	fmt.Printf("reproduced: %t\n", result.Reproduced)
	if result.OutcomeCategory != "" {
		fmt.Printf("outcome_category: %s\n", result.OutcomeCategory)
	}
	if result.OutcomeReason != "" {
		fmt.Printf("outcome_reason: %s\n", result.OutcomeReason)
	}
	fmt.Printf("expected: %s\n", result.ExpectedSignature.String())
	fmt.Printf("actual: %s\n", result.ActualSignature.String())
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func printContainerImage(image string) {
	if image != "" {
		fmt.Printf("container_image: %s\n", image)
	}
}

func printFaultPlan(faultPlanID string) {
	if faultPlanID != "" {
		fmt.Printf("fault_plan: %s\n", faultPlanID)
	}
}

func printPrimitive(primitiveID string) {
	if primitiveID != "" {
		fmt.Printf("primitive: %s\n", primitiveID)
	}
}

func printTimingProfile(timingProfileID string) {
	if timingProfileID != "" {
		fmt.Printf("timing_profile: %s\n", timingProfileID)
	}
}
