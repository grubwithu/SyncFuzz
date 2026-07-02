package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz"
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
		target(os.Args[2:])
	case "corpus":
		corpus(os.Args[2:])
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
  syncfuzz target groups
  syncfuzz target run [--command '<agent command>' | --command-file examples/target-commands/orphan-process.sh] [--target local-agent] [--task orphan-process|orphan-process-long-delay|persistent-shell-poisoning|persistent-shell-poisoning-replay|persistent-shell-poisoning-fork|file-residue-fork|delete-residue-fork|symlink-residue-fork] [--prompt-file task.md] [--expect-files late-effect] [--timeout 2m] [--observe-delay 500ms] [--late-observe-delay 7s] [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz target suite [--command '<agent command>' | --command-file examples/target-commands/orphan-process.sh] [--target local-agent] [--task orphan-process] [--tasks orphan-process,persistent-shell-poisoning,persistent-shell-poisoning-replay,persistent-shell-poisoning-fork,file-residue-fork,delete-residue-fork,symlink-residue-fork] [--group workspace-residue] [--groups phase5a-baseline] [--repeat 3] [--timeout 2m] [--observe-delay 500ms] [--late-observe-delay 7s] [--out runs] [--corpus corpus] [--env local] [--container-image ubuntu:latest]
  syncfuzz corpus list [--corpus corpus] [--limit 20]
  syncfuzz corpus show --id <entry_id> [--corpus corpus]
  syncfuzz corpus verify [--corpus corpus] [--out runs] [--limit 0] [--env local] [--container-image ubuntu:latest]
  syncfuzz replay --id <entry_id> [--corpus corpus] [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz version

`, version)
}

func list() {
	for _, c := range syncfuzz.Cases() {
		fmt.Printf("%-30s %s\n", c.Name, c.Description)
	}
}

func faultPlans() {
	fmt.Printf("%-58s %-28s %-5s %-28s %s\n", "id", "case", "phase", "impact", "fault")
	for _, plan := range syncfuzz.FaultPlans() {
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
	for _, profile := range syncfuzz.TimingProfiles() {
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
	for _, primitive := range syncfuzz.MutationPrimitives() {
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

	result, err := syncfuzz.BuildScheduleMatrix(syncfuzz.MatrixOptions{
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
	runRole := fs.String("role", syncfuzz.RunRoleFault, "run role: fault or control")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	opts := syncfuzz.RunOptions{
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
	result, err := syncfuzz.Run(context.Background(), opts)
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

	result, err := syncfuzz.RunPair(context.Background(), syncfuzz.PairOptions{
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

	result, err := syncfuzz.RunSuite(context.Background(), syncfuzz.SuiteOptions{
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

	result, err := syncfuzz.RunCampaign(context.Background(), syncfuzz.CampaignOptions{
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

func target(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "missing target subcommand: list, tasks, groups, run, or suite")
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		targetList()
	case "tasks":
		targetTasks()
	case "groups":
		targetGroups()
	case "run":
		targetRun(args[1:])
	case "suite":
		targetSuite(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown target subcommand: %s\n", args[0])
		os.Exit(2)
	}
}

func targetList() {
	fmt.Printf("%-14s %-7s %-48s %s\n", "adapter", "ready", "capabilities", "description")
	for _, adapter := range syncfuzz.TargetAdapters() {
		fmt.Printf("%-14s %-7t %-48s %s\n",
			adapter.AdapterID,
			adapter.Implemented,
			strings.Join(adapter.Capabilities, ","),
			adapter.Description,
		)
	}
}

func targetTasks() {
	fmt.Printf("%-28s %-5s %-28s %s\n", "task", "late", "default_expected", "description")
	for _, task := range syncfuzz.TargetTasks() {
		fmt.Printf("%-28s %-5t %-28s %s\n",
			task.TaskID,
			task.UsesLateObservation,
			strings.Join(task.DefaultExpectedFiles, ","),
			task.Description,
		)
	}
}

func targetGroups() {
	fmt.Printf("%-22s %-60s %s\n", "group", "tasks", "description")
	for _, group := range syncfuzz.TargetTaskGroups() {
		fmt.Printf("%-22s %-60s %s\n",
			group.GroupID,
			strings.Join(group.Tasks, ","),
			group.Description,
		)
	}
}

func targetRun(args []string) {
	fs := flag.NewFlagSet("target run", flag.ExitOnError)
	adapterID := fs.String("adapter", "command", "target adapter id")
	targetID := fs.String("target", "command", "human-readable target runtime id")
	taskID := fs.String("task", "orphan-process", "target task id")
	objective := fs.String("objective", "", "optional target objective")
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

	result, err := syncfuzz.RunTarget(context.Background(), syncfuzz.TargetRunOptions{
		AdapterID:        *adapterID,
		TargetID:         *targetID,
		TaskID:           *taskID,
		Objective:        *objective,
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
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	fmt.Printf("completed: %t\n", result.Completed)
	fmt.Printf("expectations_met: %t\n", result.ExpectationsMet)
	fmt.Printf("target_oracle: %s\n", result.TargetOracle.Name)
	fmt.Printf("oracle_confirmed: %t\n", result.TargetOracle.Confirmed)
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
	taskGroup := fs.String("group", "", "single built-in target task group to expand into suite tasks")
	taskGroups := fs.String("groups", "", "comma-separated built-in target task groups to expand before explicit tasks")
	objective := fs.String("objective", "", "optional shared objective override")
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
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	groups := splitCSV(*taskGroups)
	if *taskGroup != "" {
		groups = append([]string{*taskGroup}, groups...)
	}
	tasks := splitCSV(*taskList)
	if len(tasks) == 0 && len(groups) == 0 {
		tasks = []string{*taskID}
	} else if len(tasks) == 0 && *taskID != "orphan-process" {
		tasks = []string{*taskID}
	}

	result, err := syncfuzz.RunTargetSuite(context.Background(), syncfuzz.TargetSuiteOptions{
		AdapterID:        *adapterID,
		TargetID:         *targetID,
		Tasks:            tasks,
		TaskGroups:       groups,
		Objective:        *objective,
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
	fmt.Printf("corpus_entries: %d\n", len(result.CorpusEntries))
	fmt.Printf("artifacts: %s\n", result.ArtifactDir)
}

func corpus(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "missing corpus subcommand: list, show, or verify")
		os.Exit(2)
	}

	switch args[0] {
	case "list":
		corpusList(args[1:])
	case "show":
		corpusShow(args[1:])
	case "verify":
		corpusVerify(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown corpus subcommand: %s\n", args[0])
		os.Exit(2)
	}
}

func corpusList(args []string) {
	fs := flag.NewFlagSet("corpus list", flag.ExitOnError)
	corpusDir := fs.String("corpus", "corpus", "corpus directory")
	limit := fs.Int("limit", 20, "maximum entries to print; 0 means all")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	entries, err := syncfuzz.ListCorpus(*corpusDir, *limit)
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

	entry, err := syncfuzz.ShowCorpusEntry(*corpusDir, *entryID)
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

	result, err := syncfuzz.VerifyCorpus(context.Background(), syncfuzz.VerifyOptions{
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

	result, err := syncfuzz.ReplayCorpusEntry(context.Background(), syncfuzz.ReplayOptions{
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
