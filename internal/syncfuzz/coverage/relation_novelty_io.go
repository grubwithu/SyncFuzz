package coverage

import (
	"encoding/json"
	"fmt"
	"os"
)

func ReadRelationNoveltyLedger(path string) (RelationNoveltyLedger, error) {
	file, err := os.Open(path)
	if err != nil {
		return RelationNoveltyLedger{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	var ledger RelationNoveltyLedger
	if err := json.NewDecoder(file).Decode(&ledger); err != nil {
		return RelationNoveltyLedger{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := ledger.Validate(); err != nil {
		return RelationNoveltyLedger{}, fmt.Errorf("read %s: %w", path, err)
	}
	return ledger, nil
}

func WriteRelationNoveltyLedger(path string, ledger RelationNoveltyLedger) error {
	if err := ledger.Validate(); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(ledger); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
