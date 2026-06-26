package syncfuzz

import "testing"

func TestPrimitiveCatalogIsValid(t *testing.T) {
	if err := validatePrimitiveCatalog(); err != nil {
		t.Fatalf("primitive catalog should be valid: %v", err)
	}
}

func TestPrimitivesForCaseFiltersPlanned(t *testing.T) {
	implemented := primitivesForCase("partial-filesystem-rollback", false)
	withPlanned := primitivesForCase("partial-filesystem-rollback", true)

	if len(implemented) == 0 {
		t.Fatalf("expected implemented partial-filesystem-rollback primitives")
	}
	if len(withPlanned) <= len(implemented) {
		t.Fatalf("expected include-planned to add candidates: implemented=%d planned=%d", len(implemented), len(withPlanned))
	}
	for _, primitive := range implemented {
		if !primitive.Implemented {
			t.Fatalf("unexpected planned primitive in implemented set: %#v", primitive)
		}
	}
}

func TestBuildScheduleMatrixIncludesExecutableDoubleFork(t *testing.T) {
	matrix, err := BuildScheduleMatrix(MatrixOptions{
		Cases:            []string{"orphan-process"},
		TimingProfileIDs: []string{"baseline"},
	})
	if err != nil {
		t.Fatalf("BuildScheduleMatrix failed: %v", err)
	}
	var found bool
	for _, candidate := range matrix.Candidates {
		if candidate.PrimitiveID == "double-fork-daemon" {
			found = true
			if !candidate.Implemented {
				t.Fatalf("expected double-fork candidate to be executable")
			}
		}
	}
	if !found {
		t.Fatalf("expected double-fork-daemon in executable orphan-process matrix: %#v", matrix.Candidates)
	}
}

func TestBuildScheduleMatrixEnumeratesCandidates(t *testing.T) {
	matrix, err := BuildScheduleMatrix(MatrixOptions{
		Cases:            []string{"action-replay"},
		TimingProfileIDs: []string{"baseline", "tight"},
	})
	if err != nil {
		t.Fatalf("BuildScheduleMatrix failed: %v", err)
	}
	if matrix.SchemaVersion != "syncfuzz.schedule-matrix.v1" {
		t.Fatalf("unexpected schema version %q", matrix.SchemaVersion)
	}
	if matrix.TotalCandidates != 2 {
		t.Fatalf("expected two candidates, got %d", matrix.TotalCandidates)
	}
	for _, candidate := range matrix.Candidates {
		if candidate.CaseName != "action-replay" {
			t.Fatalf("unexpected case %q", candidate.CaseName)
		}
		if candidate.PrimitiveID != "external-api-commit" {
			t.Fatalf("unexpected primitive %q", candidate.PrimitiveID)
		}
		if !candidate.Implemented {
			t.Fatalf("expected implemented candidate")
		}
	}
}

func TestBuildScheduleMatrixRejectsUnknownTiming(t *testing.T) {
	_, err := BuildScheduleMatrix(MatrixOptions{
		Cases:            []string{"action-replay"},
		TimingProfileIDs: []string{"not-a-profile"},
	})
	if err == nil {
		t.Fatalf("expected unknown timing profile error")
	}
}
