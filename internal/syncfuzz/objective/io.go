package objective

import (
	"encoding/json"
	"fmt"
	"os"
)

func ReadStateObjective(path string) (StateObjective, error) {
	var value StateObjective
	if err := readJSON(path, &value); err != nil {
		return StateObjective{}, err
	}
	return value, value.Validate()
}

func ReadProfileRun(path string) (ProfileRun, error) {
	var value ProfileRun
	if err := readJSON(path, &value); err != nil {
		return ProfileRun{}, err
	}
	return value, nil
}

func ReadStateSeed(path string) (StateSeed, error) {
	var value StateSeed
	if err := readJSON(path, &value); err != nil {
		return StateSeed{}, err
	}
	return value, value.Validate()
}

func WriteStateSeed(path string, seed StateSeed) error {
	if err := seed.Validate(); err != nil {
		return err
	}
	return writeJSON(path, seed)
}

func WriteProfileRun(path string, run ProfileRun) error {
	if run.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported profile run schema %q", run.SchemaVersion)
	}
	return writeJSON(path, run)
}

func readJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
