package core

import (
	"encoding/json"
	"fmt"
	"os"
)

func WriteJSON(path string, value any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// EventWriter writes newline-delimited JSON so long runs can be streamed,
// diffed, and minimized without loading the whole trace at once.
type EventWriter struct {
	file *os.File
	enc  *json.Encoder
}

func NewEventWriter(path string) (*EventWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create trace %s: %w", path, err)
	}
	return &EventWriter{file: f, enc: json.NewEncoder(f)}, nil
}

func (w *EventWriter) Write(event Event) error {
	if err := w.enc.Encode(event); err != nil {
		return fmt.Errorf("write trace event: %w", err)
	}
	return nil
}

func (w *EventWriter) Close() error {
	return w.file.Close()
}
