package syncfuzz

import "time"

type Event struct {
	RunID      string         `json:"run_id"`
	CaseName   string         `json:"case_name"`
	TurnID     string         `json:"turn_id,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Phase      string         `json:"phase"`
	EventType  string         `json:"event_type"`
	Timestamp  time.Time      `json:"timestamp"`
	Detail     map[string]any `json:"detail,omitempty"`
}

// newEvent attaches common run identity to every lifecycle and probe event.
// The phase labels follow the roadmap's P0..P8 fault injection vocabulary.
func newEvent(run *runContext, phase string, eventType string, detail map[string]any) Event {
	return Event{
		RunID:      run.runID,
		CaseName:   run.caseName,
		TurnID:     run.turnID,
		ToolCallID: run.toolCallID,
		Phase:      phase,
		EventType:  eventType,
		Timestamp:  time.Now().UTC(),
		Detail:     detail,
	}
}
