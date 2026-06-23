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
	case "run":
		run(os.Args[2:])
	case "suite":
		suite(os.Args[2:])
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
  syncfuzz run --case orphan-process [--out runs] [--delay 1500ms] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case action-replay [--out runs] [--mock-url http://127.0.0.1:8910] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case authority-resurrection [--out runs] [--mock-url http://127.0.0.1:8910] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case persistent-shell-poisoning [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case partial-filesystem-rollback [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz run --case branch-leakage [--out runs] [--env local] [--container-image ubuntu:latest]
  syncfuzz suite [--out runs] [--repeat 1] [--corpus corpus] [--cases orphan-process,branch-leakage] [--env local] [--container-image ubuntu:latest]
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

func run(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	caseName := fs.String("case", "orphan-process", "testcase to execute")
	outDir := fs.String("out", "runs", "directory for run artifacts")
	delay := fs.Duration("delay", 1500*time.Millisecond, "delay before the recovery snapshot")
	workspace := fs.String("workspace", "", "optional workspace; defaults to runs/<run_id>/workspace")
	mockURL := fs.String("mock-url", "", "optional EffectServer/AuthorityServer base URL")
	envKind := fs.String("env", "local", "execution environment backend")
	containerImage := fs.String("container-image", "ubuntu:latest", "container backend image")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	opts := syncfuzz.RunOptions{
		CaseName:       *caseName,
		OutDir:         *outDir,
		Workspace:      *workspace,
		Delay:          *delay,
		MockURL:        *mockURL,
		EnvKind:        *envKind,
		ContainerImage: *containerImage,
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
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
	fmt.Printf("confirmed: %t\n", result.Confirmed)
	fmt.Printf("signature: %s\n", result.Signature.String())
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
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := syncfuzz.RunSuite(context.Background(), syncfuzz.SuiteOptions{
		OutDir:         *outDir,
		Repeat:         *repeat,
		Cases:          splitCases(*caseList),
		Delay:          *delay,
		MockURL:        *mockURL,
		CorpusDir:      *corpusDir,
		EnvKind:        *envKind,
		ContainerImage: *containerImage,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz suite failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("suite_id: %s\n", result.SuiteID)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
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
	fmt.Printf("%-48s %-18s %-28s %-5s %s\n", "entry_id", "kind", "case", "score", "signature")
	for _, entry := range entries {
		fmt.Printf("%-48s %-18s %-28s %-5d %s\n",
			entry.EntryID,
			entry.Kind,
			entry.CaseName,
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
	fmt.Printf("kind: %s\n", entry.Kind)
	fmt.Printf("score: %d\n", entry.Score)
	fmt.Printf("case: %s\n", entry.CaseName)
	fmt.Printf("suite_id: %s\n", entry.SuiteID)
	fmt.Printf("run_id: %s\n", entry.RunID)
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
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := syncfuzz.VerifyCorpus(context.Background(), syncfuzz.VerifyOptions{
		CorpusDir:      *corpusDir,
		OutDir:         *outDir,
		Limit:          *limit,
		Delay:          *delay,
		MockURL:        *mockURL,
		EnvKind:        *envKind,
		ContainerImage: *containerImage,
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
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	result, err := syncfuzz.ReplayCorpusEntry(context.Background(), syncfuzz.ReplayOptions{
		CorpusDir:      *corpusDir,
		EntryID:        *entryID,
		OutDir:         *outDir,
		Delay:          *delay,
		MockURL:        *mockURL,
		EnvKind:        *envKind,
		ContainerImage: *containerImage,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncfuzz replay failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("replay_id: %s\n", result.ReplayID)
	fmt.Printf("entry_id: %s\n", result.EntryID)
	fmt.Printf("case: %s\n", result.CaseName)
	fmt.Printf("environment: %s\n", result.Environment)
	printContainerImage(result.ContainerImage)
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
