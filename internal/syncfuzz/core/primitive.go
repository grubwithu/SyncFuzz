package core

import (
	"fmt"
	"sort"
	"strings"
)

type MutationPrimitive struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	StateLayers  []string `json:"state_layers"`
	StateClasses []string `json:"state_classes"`
	Phases       []string `json:"phases"`
	CaseNames    []string `json:"case_names,omitempty"`
	Implemented  bool     `json:"implemented"`
	Description  string   `json:"description"`
}

func MutationPrimitives() []MutationPrimitive {
	return []MutationPrimitive{
		{
			ID:           "background-process",
			Name:         "Background process",
			Category:     "process",
			StateLayers:  []string{"os"},
			StateClasses: []string{"process"},
			Phases:       []string{"P3", "P5", "P6"},
			CaseNames:    []string{"orphan-process"},
			Implemented:  true,
			Description:  "Spawn a process that can outlive the agent-visible tool boundary.",
		},
		{
			ID:           "delayed-write",
			Name:         "Delayed write",
			Category:     "filesystem",
			StateLayers:  []string{"os"},
			StateClasses: []string{"filesystem"},
			Phases:       []string{"P5", "P6"},
			CaseNames:    []string{"orphan-process"},
			Implemented:  true,
			Description:  "Materialize filesystem state after the command return boundary.",
		},
		{
			ID:           "path-cwd-modification",
			Name:         "PATH and cwd modification",
			Category:     "shell",
			StateLayers:  []string{"os"},
			StateClasses: []string{"shell-state", "filesystem"},
			Phases:       []string{"P4", "P6", "P8"},
			CaseNames:    []string{"persistent-shell-poisoning"},
			Implemented:  true,
			Description:  "Change process-local shell state that graph checkpoints may not capture.",
		},
		{
			ID:           "shell-alias-function",
			Name:         "Shell alias or function",
			Category:     "shell",
			StateLayers:  []string{"os"},
			StateClasses: []string{"shell-state"},
			Phases:       []string{"P4", "P6", "P8"},
			CaseNames:    []string{"persistent-shell-poisoning"},
			Implemented:  true,
			Description:  "Leave command-resolution behavior in a reused persistent shell.",
		},
		{
			ID:           "untracked-file",
			Name:         "Untracked file",
			Category:     "filesystem",
			StateLayers:  []string{"os"},
			StateClasses: []string{"filesystem"},
			Phases:       []string{"P4", "P6"},
			CaseNames:    []string{"partial-filesystem-rollback"},
			Implemented:  true,
			Description:  "Create filesystem objects outside a tracked rollback set.",
		},
		{
			ID:           "symlink",
			Name:         "Symlink",
			Category:     "filesystem",
			StateLayers:  []string{"os"},
			StateClasses: []string{"filesystem"},
			Phases:       []string{"P4", "P6"},
			CaseNames:    []string{"partial-filesystem-rollback"},
			Implemented:  true,
			Description:  "Use symlink state that content-only rollback can miss.",
		},
		{
			ID:           "chmod-xattr",
			Name:         "chmod or xattr drift",
			Category:     "filesystem",
			StateLayers:  []string{"os"},
			StateClasses: []string{"filesystem-metadata"},
			Phases:       []string{"P4", "P6"},
			CaseNames:    []string{"partial-filesystem-rollback"},
			Implemented:  true,
			Description:  "Mutate metadata that byte-level restore does not reset.",
		},
		{
			ID:           "external-api-commit",
			Name:         "External API commit",
			Category:     "external",
			StateLayers:  []string{"external"},
			StateClasses: []string{"external-effect"},
			Phases:       []string{"P4", "P5", "P8"},
			CaseNames:    []string{"action-replay"},
			Implemented:  true,
			Description:  "Commit an external effect that cannot be rolled back by agent state.",
		},
		{
			ID:           "single-use-capability",
			Name:         "Single-use capability",
			Category:     "authority",
			StateLayers:  []string{"authority"},
			StateClasses: []string{"authority-state"},
			Phases:       []string{"P4", "P6", "P8"},
			CaseNames:    []string{"authority-resurrection"},
			Implemented:  true,
			Description:  "Consume authority state once, then test stale replay semantics.",
		},
		{
			ID:           "branch-artifact-residue",
			Name:         "Branch artifact residue",
			Category:     "branch",
			StateLayers:  []string{"agent", "os"},
			StateClasses: []string{"filesystem"},
			Phases:       []string{"P4", "P6", "P8"},
			CaseNames:    []string{"branch-leakage"},
			Implemented:  true,
			Description:  "Let discarded branch effects remain in the committed workspace.",
		},
		{
			ID:           "double-fork-daemon",
			Name:         "Double-fork daemon",
			Category:     "process",
			StateLayers:  []string{"os"},
			StateClasses: []string{"process"},
			Phases:       []string{"P3", "P5", "P6"},
			CaseNames:    []string{"orphan-process"},
			Implemented:  true,
			Description:  "Daemonize process state that can survive the agent-visible command boundary.",
		},
		{
			ID:           "open-fd",
			Name:         "Open file descriptor",
			Category:     "process",
			StateLayers:  []string{"os"},
			StateClasses: []string{"fd"},
			Phases:       []string{"P4", "P6"},
			CaseNames:    []string{"partial-filesystem-rollback"},
			Implemented:  true,
			Description:  "Keep a deleted workspace inode reachable through an open file descriptor after rollback.",
		},
		{
			ID:           "unix-socket",
			Name:         "Unix socket",
			Category:     "process",
			StateLayers:  []string{"os"},
			StateClasses: []string{"socket"},
			Phases:       []string{"P4", "P6"},
			CaseNames:    []string{"persistent-shell-poisoning"},
			Implemented:  false,
			Description:  "Planned primitive for local IPC residue.",
		},
		{
			ID:           "concurrent-file-replacement",
			Name:         "Concurrent file replacement",
			Category:     "filesystem",
			StateLayers:  []string{"os"},
			StateClasses: []string{"filesystem"},
			Phases:       []string{"P4", "P5", "P6"},
			CaseNames:    []string{"partial-filesystem-rollback"},
			Implemented:  false,
			Description:  "Planned primitive for race-shaped filesystem drift.",
		},
	}
}

func PrimitiveByID(id string) (MutationPrimitive, bool) {
	for _, primitive := range MutationPrimitives() {
		if primitive.ID == id {
			return primitive, true
		}
	}
	return MutationPrimitive{}, false
}

func PrimitivesForCase(caseName string, includePlanned bool) []MutationPrimitive {
	var out []MutationPrimitive
	for _, primitive := range MutationPrimitives() {
		if !includePlanned && !primitive.Implemented {
			continue
		}
		if StringInSlice(caseName, primitive.CaseNames) {
			out = append(out, primitive)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func ValidatePrimitiveCatalog() error {
	knownCases := make(map[string]struct{})
	for _, name := range CaseNames() {
		knownCases[name] = struct{}{}
	}
	seen := make(map[string]struct{})
	for _, primitive := range MutationPrimitives() {
		if strings.TrimSpace(primitive.ID) == "" {
			return fmt.Errorf("primitive id is required")
		}
		if _, ok := seen[primitive.ID]; ok {
			return fmt.Errorf("duplicate primitive id %q", primitive.ID)
		}
		seen[primitive.ID] = struct{}{}
		for _, caseName := range primitive.CaseNames {
			if _, ok := knownCases[caseName]; !ok {
				return fmt.Errorf("primitive %q references unknown case %q", primitive.ID, caseName)
			}
		}
	}
	return nil
}

func StringInSlice(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
