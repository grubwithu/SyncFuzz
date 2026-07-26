package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/environment"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/hazard"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/recovery"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/synthesis"
	"github.com/grubwithu/syncfuzz/internal/syncfuzz/target"
)

func runHazard(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "syncfuzz hazard requires a subcommand; supported: unix-socket-calibration, langgraph-target-report")
		os.Exit(2)
	}
	switch args[0] {
	case "unix-socket-calibration":
		hazardUnixSocketCalibration(args[1:])
	case "langgraph-target-report":
		hazardLangGraphTargetReport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown syncfuzz hazard subcommand %q\n", args[0])
		os.Exit(2)
	}
}

// hazardLangGraphTargetReport joins two independently profiled target runs:
// one explicit rebind treatment and one clean non-rebinding control. It only
// consumes immutable artifacts and fails closed when the runs cannot support
// a comparable five-control report.
func hazardLangGraphTargetReport(args []string) {
	fs := flag.NewFlagSet("hazard langgraph-target-report", flag.ExitOnError)
	candidatePath := fs.String("candidate", "", "synthesis candidate JSON shared by tainted and clean runs")
	baseProjectID := fs.String("base-project-id", "", "stable identifier of the normal base project")
	runnerConstraints := fs.String("runner-constraints", "", "frozen image/model/tool/timeout constraints")
	taintedSeedPath := fs.String("tainted-seed", "", "tainted StateSeed JSON")
	taintedSetPath := fs.String("tainted-set", "", "tainted historical recovery set JSON")
	taintedExecutionPath := fs.String("tainted-execution", "", "tainted fork recovery-set execution JSON")
	taintedProgramPath := fs.String("tainted-program", "", "tainted EnvironmentProgram JSON")
	taintedMaterializationPath := fs.String("tainted-materialization", "", "optional tainted materialization JSON; defaults beside tainted seed's recorded plan")
	cleanSeedPath := fs.String("clean-seed", "", "clean StateSeed JSON")
	cleanSetPath := fs.String("clean-set", "", "clean historical recovery set JSON")
	cleanExecutionPath := fs.String("clean-execution", "", "clean fork recovery-set execution JSON")
	cleanProgramPath := fs.String("clean-program", "", "clean EnvironmentProgram JSON")
	cleanMaterializationPath := fs.String("clean-materialization", "", "optional clean materialization JSON; defaults beside clean seed's recorded plan")
	outPath := fs.String("out", "", "output RecoveryHazardReport JSON")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	required := map[string]string{
		"--candidate": *candidatePath, "--base-project-id": *baseProjectID, "--runner-constraints": *runnerConstraints,
		"--tainted-seed": *taintedSeedPath, "--tainted-set": *taintedSetPath, "--tainted-execution": *taintedExecutionPath, "--tainted-program": *taintedProgramPath,
		"--clean-seed": *cleanSeedPath, "--clean-set": *cleanSetPath, "--clean-execution": *cleanExecutionPath, "--clean-program": *cleanProgramPath,
		"--out": *outPath,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			fmt.Fprintf(os.Stderr, "syncfuzz hazard langgraph-target-report requires %s\n", name)
			os.Exit(2)
		}
	}
	candidate, err := synthesis.ReadCandidate(*candidatePath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	taintedSeed, err := objective.ReadStateSeed(*taintedSeedPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	taintedSet, err := recovery.ReadHistoricalRecoverySet(*taintedSetPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	taintedExecution, err := recovery.ReadForkRecoverySetExecution(*taintedExecutionPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	taintedProgram, err := environment.ReadEnvironmentProgram(*taintedProgramPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	taintedMaterialization, err := readTargetMaterializationForSeed(taintedSeed, *taintedMaterializationPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	cleanSeed, err := objective.ReadStateSeed(*cleanSeedPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	cleanSet, err := recovery.ReadHistoricalRecoverySet(*cleanSetPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	cleanExecution, err := recovery.ReadForkRecoverySetExecution(*cleanExecutionPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	cleanProgram, err := environment.ReadEnvironmentProgram(*cleanProgramPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	cleanMaterialization, err := readTargetMaterializationForSeed(cleanSeed, *cleanMaterializationPath)
	if err != nil {
		hazardTargetReportFailure(err)
	}
	if candidate.SchemaVersion != synthesis.SchemaVersion || candidate.CandidateID != taintedSeed.SynthesisCandidateID || candidate.CandidateID != cleanSeed.SynthesisCandidateID || candidate.TargetID != "langgraph-shell-react" || candidate.AdapterID != recovery.LangGraphForkAdapterID {
		hazardTargetReportFailure(fmt.Errorf("candidate does not match the two LangGraph target StateSeeds"))
	}
	if taintedSet.ContinuationQuery == nil {
		hazardTargetReportFailure(fmt.Errorf("tainted recovery set has no frozen continuation query"))
	}
	workload, err := hazard.NewWorkload(hazard.WorkloadOptions{
		BaseProjectID: *baseProjectID, InitialPrompt: candidate.Task, ContinuationPrompt: taintedSet.ContinuationQuery.Query, RunnerConstraints: *runnerConstraints,
	})
	if err != nil {
		hazardTargetReportFailure(err)
	}
	report, err := hazard.BuildLangGraphTargetRecoveryHazardReport(hazard.LangGraphTargetHazardInput{
		Workload:    workload,
		TaintedSeed: taintedSeed, TaintedRecoverySet: taintedSet, TaintedRecoveryExecution: taintedExecution, TaintedProgram: taintedProgram, TaintedMaterialization: taintedMaterialization,
		CleanSeed: cleanSeed, CleanRecoverySet: cleanSet, CleanRecoveryExecution: cleanExecution, CleanProgram: cleanProgram, CleanMaterialization: cleanMaterialization,
	})
	if err != nil {
		hazardTargetReportFailure(err)
	}
	if err := hazard.WriteRecoveryHazardReport(*outPath, report); err != nil {
		hazardTargetReportFailure(err)
	}
	fmt.Printf("hazard_report_id: %s\n", report.ReportID)
	fmt.Printf("hazard_status: %s\n", report.Status)
	fmt.Printf("hazard_class: %s\n", report.Class)
	fmt.Printf("artifact: %s\n", *outPath)
}

func readTargetMaterializationForSeed(seed objective.StateSeed, explicitPath string) (environment.TargetUnixSocketMaterialization, error) {
	path := strings.TrimSpace(explicitPath)
	if path == "" {
		if strings.TrimSpace(seed.RecordedPlanArtifact) == "" {
			return environment.TargetUnixSocketMaterialization{}, fmt.Errorf("StateSeed %q has no recorded plan artifact from which to locate target materialization", seed.SeedID)
		}
		path = filepath.Join(filepath.Dir(seed.RecordedPlanArtifact), target.TargetEnvironmentMaterializationArtifact)
	}
	return environment.ReadTargetUnixSocketMaterialization(path)
}

func hazardTargetReportFailure(err error) {
	fmt.Fprintf(os.Stderr, "syncfuzz hazard langgraph-target-report failed: %v\n", err)
	os.Exit(1)
}

func hazardUnixSocketCalibration(args []string) {
	fs := flag.NewFlagSet("hazard unix-socket-calibration", flag.ExitOnError)
	workspace := fs.String("workspace", "", "optional scratch directory; a short temporary socket root is used when required")
	timeout := fs.Duration("timeout", 30*time.Second, "maximum fixture calibration duration")
	outPath := fs.String("out", "unix-socket-recovery-hazard-calibration.json", "fixture calibration JSON output path")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *timeout <= 0 || strings.TrimSpace(*outPath) == "" {
		fmt.Fprintln(os.Stderr, "syncfuzz hazard unix-socket-calibration requires a positive --timeout and --out")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := hazard.RunUnixSocketCalibration(ctx, *workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz hazard unix-socket-calibration failed: %v\n", err)
		os.Exit(1)
	}
	if err := hazard.WriteUnixSocketCalibrationResult(*outPath, result); err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz hazard unix-socket-calibration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("calibration_id: %s\n", result.CalibrationID)
	fmt.Printf("scope: %s\n", result.Scope)
	fmt.Printf("frontier_id: %s\n", result.HazardReport.RecoveryEvidence.FrontierID)
	fmt.Printf("hazard_class: %s\n", result.HazardReport.Class)
	fmt.Printf("hazard_status: %s\n", result.HazardReport.Status)
	fmt.Printf("artifact: %s\n", *outPath)
}
