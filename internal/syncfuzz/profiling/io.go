package profiling

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func ReadCheckpointCatalog(path string) (CheckpointCatalog, error) {
	var catalog CheckpointCatalog
	if err := readJSON(path, &catalog); err != nil {
		return CheckpointCatalog{}, err
	}
	return catalog, catalog.Validate()
}

func ReadRawEventsJSONL(path string) ([]RawEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open raw event trace: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	events := make([]RawEvent, 0)
	for line := 1; scanner.Scan(); line++ {
		var event RawEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode raw event trace line %d: %w", line, err)
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("validate raw event trace line %d: %w", line, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read raw event trace: %w", err)
	}
	return events, nil
}

func ReadCheckpointStateSummaries(path string) ([]CheckpointStateSummary, error) {
	var summaries []CheckpointStateSummary
	if err := readJSON(path, &summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

func ReadCheckpointEffectMap(path string) (CheckpointEffectMap, error) {
	var result CheckpointEffectMap
	if err := readJSON(path, &result); err != nil {
		return CheckpointEffectMap{}, err
	}
	return result, result.Validate()
}

func WriteCheckpointEffectMap(path string, result CheckpointEffectMap) error {
	return writeJSON(path, result)
}

func WriteNormalizedEffects(path string, effects []NormalizedEffect) error {
	return writeJSON(path, effects)
}

func WriteRawEventsJSONL(path string, events []RawEvent) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return fmt.Errorf("write raw event trace %s: %w", path, err)
		}
	}
	return nil
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
