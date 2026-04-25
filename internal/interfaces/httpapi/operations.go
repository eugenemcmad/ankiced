package httpapi

import (
	"sync"
	"time"

	"ankiced/internal/domain"
	"ankiced/internal/presentation"
)

type operationStatus string

const (
	operationRunning   operationStatus = "running"
	operationSucceeded operationStatus = "succeeded"
	operationFailed    operationStatus = "failed"
)

type operationState struct {
	ID         string                 `json:"operation_id"`
	Kind       string                 `json:"kind"`
	Status     operationStatus        `json:"status"`
	StartedAt  string                 `json:"started_at"`
	FinishedAt string                 `json:"finished_at,omitempty"`
	Progress   domain.CleanerProgress `json:"progress"`
	Summary    domain.DryRunSummary   `json:"summary,omitempty"`
	Output     string                 `json:"output,omitempty"`
	Error      *apiErrorPayload       `json:"error,omitempty"`
}

type operationStore struct {
	mu    sync.Mutex
	items map[string]operationState
}

func newOperationStore() *operationStore {
	return &operationStore{items: make(map[string]operationState)}
}

func (s *operationStore) create(id, kind string) operationState {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := operationState{
		ID:        id,
		Kind:      kind,
		Status:    operationRunning,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.items[id] = op
	return op
}

func (s *operationStore) get(id string) (operationState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.items[id]
	return op, ok
}

func (s *operationStore) succeed(id, output string, summary domain.DryRunSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := s.items[id]
	op.Status = operationSucceeded
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	op.Output = output
	op.Summary = summary
	op.Progress = domain.CleanerProgress{
		Total:     op.Progress.Total,
		Processed: summary.Processed,
		Changed:   summary.Changed,
		Skipped:   summary.Skipped,
		Errors:    summary.Errors,
		Stage:     "finished",
		Summary:   summary,
	}
	s.items[id] = op
}

func (s *operationStore) fail(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := s.items[id]
	op.Status = operationFailed
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	progress := op.Progress
	progress.Stage = "failed"
	progress.Errors++
	op.Progress = progress
	op.Error = &apiErrorPayload{
		Message:           presentation.FormatError(err),
		RecommendedAction: recommendedAction(mapErrorCode(err)),
		Code:              mapErrorCode(err),
		Details:           presentation.FormatDebugError(err),
		CorrelationID:     id,
	}
	s.items[id] = op
}

func (s *operationStore) progress(id string, progress domain.CleanerProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := s.items[id]
	op.Progress = progress
	s.items[id] = op
}
