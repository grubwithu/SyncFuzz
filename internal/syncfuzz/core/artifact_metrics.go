package core

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

type artifactMetrics struct {
	Files int
	Bytes int64
}

func MeasureArtifactMetrics(root string) (artifactMetrics, error) {
	if root == "" {
		return artifactMetrics{}, nil
	}

	var metrics artifactMetrics
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		metrics.Files++
		metrics.Bytes += info.Size()
		return nil
	})
	if err != nil {
		return artifactMetrics{}, fmt.Errorf("measure artifact metrics for %s: %w", root, err)
	}
	return metrics, nil
}
