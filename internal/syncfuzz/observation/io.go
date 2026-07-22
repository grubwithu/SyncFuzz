package observation

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/core"
)

func ReadFootprint(path string) (ResourceFootprint, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ResourceFootprint{}, fmt.Errorf("read resource footprint %s: %w", path, err)
	}
	var footprint ResourceFootprint
	if err := json.Unmarshal(raw, &footprint); err != nil {
		return ResourceFootprint{}, fmt.Errorf("decode resource footprint %s: %w", path, err)
	}
	if err := NormalizeFootprint(&footprint); err != nil {
		return ResourceFootprint{}, err
	}
	return footprint, nil
}

func WriteFootprint(path string, footprint *ResourceFootprint) error {
	if err := NormalizeFootprint(footprint); err != nil {
		return err
	}
	if err := core.WriteJSON(path, footprint); err != nil {
		return fmt.Errorf("write resource footprint %s: %w", path, err)
	}
	return nil
}

func ReadPlan(path string) (ObservationPlan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ObservationPlan{}, fmt.Errorf("read observation plan %s: %w", path, err)
	}
	var plan ObservationPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return ObservationPlan{}, fmt.Errorf("decode observation plan %s: %w", path, err)
	}
	if err := ValidatePlan(&plan); err != nil {
		return ObservationPlan{}, err
	}
	return plan, nil
}

func WritePlan(path string, plan *ObservationPlan) error {
	if err := ValidatePlan(plan); err != nil {
		return err
	}
	if err := core.WriteJSON(path, plan); err != nil {
		return fmt.Errorf("write observation plan %s: %w", path, err)
	}
	return nil
}
