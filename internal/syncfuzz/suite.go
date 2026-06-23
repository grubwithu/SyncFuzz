package syncfuzz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SuiteOptions struct {
	OutDir         string
	Repeat         int
	Cases          []string
	Delay          time.Duration
	MockURL        string
	CorpusDir      string
	EnvKind        string
	ContainerImage string
}

type SuiteCaseResult struct {
	CaseName    string            `json:"case_name"`
	Iteration   int               `json:"iteration"`
	RunID       string            `json:"run_id,omitempty"`
	Confirmed   bool              `json:"confirmed"`
	Signature   MismatchSignature `json:"signature,omitempty"`
	Interesting bool              `json:"interesting"`
	Novelty     []string          `json:"novelty,omitempty"`
	Score       int               `json:"score"`
	ArtifactDir string            `json:"artifact_dir,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type SuiteDiscovery struct {
	Kind        string            `json:"kind"`
	Key         string            `json:"key"`
	CaseName    string            `json:"case_name"`
	Iteration   int               `json:"iteration"`
	RunID       string            `json:"run_id"`
	Signature   MismatchSignature `json:"signature"`
	ArtifactDir string            `json:"artifact_dir"`
}

type SuiteResult struct {
	SuiteID            string            `json:"suite_id"`
	StartedAt          string            `json:"started_at"`
	FinishedAt         string            `json:"finished_at"`
	ArtifactDir        string            `json:"artifact_dir"`
	Environment        string            `json:"environment"`
	ContainerImage     string            `json:"container_image,omitempty"`
	Repeat             int               `json:"repeat"`
	TotalRuns          int               `json:"total_runs"`
	Confirmed          int               `json:"confirmed"`
	Unconfirmed        int               `json:"unconfirmed"`
	Errors             int               `json:"errors"`
	UniqueSignatures   int               `json:"unique_signatures"`
	UniqueStateClasses int               `json:"unique_state_classes"`
	UniqueImpacts      int               `json:"unique_impacts"`
	Discoveries        []SuiteDiscovery  `json:"discoveries"`
	CorpusEntries      []CorpusEntry     `json:"corpus_entries,omitempty"`
	Results            []SuiteCaseResult `json:"results"`
}

// RunSuite is the first scheduler-shaped API. It still runs deterministically,
// but it gives us a stable place to add repetition, selection, baselines, and
// feedback-guided scheduling later.
func RunSuite(ctx context.Context, opts SuiteOptions) (*SuiteResult, error) {
	if opts.OutDir == "" {
		opts.OutDir = "runs"
	}
	if opts.Repeat <= 0 {
		opts.Repeat = 1
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}
	if err := validateEnvironmentKind(opts.EnvKind); err != nil {
		return nil, err
	}

	selected := opts.Cases
	if len(selected) == 0 {
		selected = caseNames()
	}
	if err := validateCaseNames(selected); err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	suiteID := fmt.Sprintf("suite-%d", started.UnixNano())
	suiteDir := filepath.Join(opts.OutDir, suiteID)
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		return nil, fmt.Errorf("create suite directory: %w", err)
	}

	result := &SuiteResult{
		SuiteID:        suiteID,
		StartedAt:      started.Format(time.RFC3339Nano),
		ArtifactDir:    suiteDir,
		Environment:    normalizedEnvKind(opts.EnvKind),
		ContainerImage: containerImageForResult(opts.EnvKind, opts.ContainerImage),
		Repeat:         opts.Repeat,
		Discoveries:    []SuiteDiscovery{},
		Results:        []SuiteCaseResult{},
	}
	feedback := newSuiteFeedback()

	for iteration := 1; iteration <= opts.Repeat; iteration++ {
		for _, caseName := range selected {
			runResult, err := Run(ctx, RunOptions{
				CaseName:       caseName,
				OutDir:         suiteDir,
				Delay:          opts.Delay,
				MockURL:        opts.MockURL,
				EnvKind:        opts.EnvKind,
				ContainerImage: opts.ContainerImage,
			})
			item := SuiteCaseResult{
				CaseName:  caseName,
				Iteration: iteration,
			}
			if err != nil {
				item.Error = err.Error()
				result.Errors++
				result.Results = append(result.Results, item)
				continue
			}

			item.RunID = runResult.RunID
			item.Confirmed = runResult.Confirmed
			item.Signature = runResult.Signature
			item.ArtifactDir = runResult.ArtifactDir
			if runResult.Confirmed {
				result.Confirmed++
				feedback.Apply(&item, result)
			} else {
				result.Unconfirmed++
			}
			result.Results = append(result.Results, item)
		}
	}

	result.TotalRuns = len(result.Results)
	result.UniqueSignatures = len(feedback.signatures)
	result.UniqueStateClasses = len(feedback.stateClasses)
	result.UniqueImpacts = len(feedback.impacts)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	corpusEntries, err := WriteCorpus(opts.CorpusDir, result)
	if err != nil {
		return nil, err
	}
	result.CorpusEntries = corpusEntries
	if err := writeJSON(filepath.Join(suiteDir, "suite-result.json"), result); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(suiteDir, "interesting.json"), result.Discoveries); err != nil {
		return nil, err
	}
	return result, nil
}

type suiteFeedback struct {
	signatures   map[string]struct{}
	stateClasses map[string]struct{}
	impacts      map[string]struct{}
}

func newSuiteFeedback() *suiteFeedback {
	return &suiteFeedback{
		signatures:   make(map[string]struct{}),
		stateClasses: make(map[string]struct{}),
		impacts:      make(map[string]struct{}),
	}
}

func (f *suiteFeedback) Apply(item *SuiteCaseResult, result *SuiteResult) {
	f.observe("new-signature", item.Signature.String(), 10, item, result, f.signatures)
	f.observe("new-state-class", item.Signature.StateClass, 3, item, result, f.stateClasses)
	f.observe("new-impact", item.Signature.Impact, 5, item, result, f.impacts)
	if item.Score > 0 {
		item.Interesting = true
	}
}

func (f *suiteFeedback) observe(kind string, key string, score int, item *SuiteCaseResult, result *SuiteResult, seen map[string]struct{}) {
	if key == "" {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	item.Novelty = append(item.Novelty, kind)
	item.Score += score
	result.Discoveries = append(result.Discoveries, SuiteDiscovery{
		Kind:        kind,
		Key:         key,
		CaseName:    item.CaseName,
		Iteration:   item.Iteration,
		RunID:       item.RunID,
		Signature:   item.Signature,
		ArtifactDir: item.ArtifactDir,
	})
}

func caseNames() []string {
	cases := Cases()
	names := make([]string, 0, len(cases))
	for _, c := range cases {
		names = append(names, c.Name)
	}
	return names
}

func validateCaseNames(names []string) error {
	known := make(map[string]struct{})
	for _, name := range caseNames() {
		known[name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("unknown case %q", name)
		}
	}
	return nil
}
