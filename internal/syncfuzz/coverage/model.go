// Package coverage records V2 state-space coverage without carrying legacy
// testcase, scenario-mutation, or query-genealogy dimensions.
package coverage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

const SchemaVersion = "syncfuzz.coverage.v1"

type Outcome string

const (
	OutcomeConsistent     Outcome = "consistent"
	OutcomeResidual       Outcome = "residual"
	OutcomeMissing        Outcome = "missing"
	OutcomeDuplicate      Outcome = "duplicate"
	OutcomeReconstruction Outcome = "reconstruction"
	OutcomeInconclusive   Outcome = "inconclusive"
)

// CoverageRecord is the exact V2 coverage tuple from the research plan.
type CoverageRecord struct {
	SchemaVersion      string                `json:"schema_version"`
	SeedID             string                `json:"seed_id"`
	ObjectiveID        string                `json:"objective_id"`
	Family             profiling.StateFamily `json:"family"`
	Operation          string                `json:"operation"`
	Lifetime           string                `json:"lifetime"`
	ResourceRelation   string                `json:"resource_relation"`
	Boundary           string                `json:"boundary"`
	CheckpointRelation string                `json:"checkpoint_relation"`
	Outcome            Outcome               `json:"outcome"`
}

func (r CoverageRecord) Validate() error {
	if r.SchemaVersion != "" && r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported coverage schema %q", r.SchemaVersion)
	}
	if strings.TrimSpace(r.SeedID) == "" || strings.TrimSpace(r.ObjectiveID) == "" {
		return fmt.Errorf("coverage record requires seed and objective IDs")
	}
	if !r.Family.Valid() || strings.TrimSpace(r.Operation) == "" || strings.TrimSpace(r.Lifetime) == "" || strings.TrimSpace(r.ResourceRelation) == "" || strings.TrimSpace(r.Boundary) == "" || strings.TrimSpace(r.CheckpointRelation) == "" {
		return fmt.Errorf("coverage record for seed %q is incomplete", r.SeedID)
	}
	switch r.Outcome {
	case OutcomeConsistent, OutcomeResidual, OutcomeMissing, OutcomeDuplicate, OutcomeReconstruction, OutcomeInconclusive:
	default:
		return fmt.Errorf("coverage record for seed %q has invalid outcome %q", r.SeedID, r.Outcome)
	}
	return nil
}

func (r CoverageRecord) TupleKey() string {
	return strings.Join([]string{
		string(r.Family), r.Operation, r.Lifetime, r.ResourceRelation,
		r.Boundary, r.CheckpointRelation, string(r.Outcome),
	}, "\x00")
}

// UniqueTuples returns a deterministic coverage view. Multiple seeds may
// support the same tuple, but they do not inflate state-space coverage.
func UniqueTuples(records []CoverageRecord) ([]CoverageRecord, error) {
	unique := make(map[string]CoverageRecord, len(records))
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return nil, err
		}
		key := record.TupleKey()
		if existing, ok := unique[key]; !ok || record.SeedID < existing.SeedID {
			unique[key] = record
		}
	}
	result := make([]CoverageRecord, 0, len(unique))
	for _, record := range unique {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TupleKey() < result[j].TupleKey() })
	return result, nil
}
