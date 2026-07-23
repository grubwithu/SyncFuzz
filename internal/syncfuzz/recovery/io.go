package recovery

import (
	"encoding/json"
	"fmt"
	"os"
)

func WriteRecoveryPair(path string, pair RecoveryPair) error {
	return writeRecoveryJSON(path, pair)
}

func ReadRecoveryPair(path string) (RecoveryPair, error) {
	file, err := os.Open(path)
	if err != nil {
		return RecoveryPair{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var pair RecoveryPair
	if err := json.NewDecoder(file).Decode(&pair); err != nil {
		return RecoveryPair{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return pair, nil
}

func writeRecoveryJSON(path string, value any) error {
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
