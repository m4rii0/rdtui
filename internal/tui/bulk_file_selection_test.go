package tui

import "testing"

func TestBulkFileSelectionPrepareNextPrompt(t *testing.T) {
	tests := []struct {
		name  string
		plans int
		start int
		want  bool
	}{
		{name: "empty", start: 0},
		{name: "first", plans: 2, start: 0, want: true},
		{name: "next", plans: 2, start: 1, want: true},
		{name: "end", plans: 2, start: 2},
		{name: "past end", plans: 2, start: 3},
		{name: "negative", plans: 2, start: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &bulkFileSelectionState{
				Plans:      make([]bulkFileSelectionPlan, tt.plans),
				Prompt:     1,
				FileCursor: 3,
			}
			if got := state.prepareNextPrompt(tt.start); got != tt.want {
				t.Fatalf("prepareNextPrompt(%d) = %v, want %v", tt.start, got, tt.want)
			}
			wantPrompt := -1
			if tt.want {
				wantPrompt = tt.start
			}
			if state.Prompt != wantPrompt || state.FileCursor != 0 {
				t.Fatalf("prompt/cursor = %d/%d, want %d/0", state.Prompt, state.FileCursor, wantPrompt)
			}
		})
	}
}

func TestBulkFileSelectionPrepareFirstPrompt(t *testing.T) {
	state := &bulkFileSelectionState{Plans: make([]bulkFileSelectionPlan, 2), Prompt: 1, FileCursor: 3}
	if !state.prepareFirstPrompt() || state.Prompt != 0 || state.FileCursor != 0 {
		t.Fatalf("prepareFirstPrompt did not reset to the first plan: %+v", state)
	}
}

func TestBulkFileSelectionPreparePromptNilState(t *testing.T) {
	var state *bulkFileSelectionState
	if state.prepareFirstPrompt() || state.prepareNextPrompt(0) {
		t.Fatal("nil state should not prepare a prompt")
	}
}
