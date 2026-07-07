package core

import "fmt"

// Cases is the public registry of known-answer synthetic testcases, used by
// both the CLI and schedulers.
func Cases() []Case {
	return []Case{
		{
			Name:        "orphan-process",
			Description: "detect a delayed OS effect that survives an agent lifecycle boundary",
		},
		{
			Name:        "action-replay",
			Description: "detect duplicated external effects after a lost response and replay",
		},
		{
			Name:        "authority-resurrection",
			Description: "detect reuse attempts for consumed single-use authority after replay",
		},
		{
			Name:        "persistent-shell-poisoning",
			Description: "detect PATH/cwd/alias residue in a reused persistent shell",
		},
		{
			Name:        "partial-filesystem-rollback",
			Description: "detect untracked, symlink, and metadata residue after naive rollback",
		},
		{
			Name:        "branch-leakage",
			Description: "detect discarded branch effects leaking into committed branch state",
		},
	}
}

func CaseNames() []string {
	cases := Cases()
	names := make([]string, 0, len(cases))
	for _, c := range cases {
		names = append(names, c.Name)
	}
	return names
}

func ValidateCaseNames(names []string) error {
	known := make(map[string]struct{})
	for _, name := range CaseNames() {
		known[name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("unknown case %q", name)
		}
	}
	return nil
}
