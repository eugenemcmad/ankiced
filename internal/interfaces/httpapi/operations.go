package httpapi

import (
	"container/list"
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

// defaultOperationStoreCapacity bounds the in-memory operation history to keep
// memory predictable for long-running web servers. Once exceeded, the oldest
// non-running operation is evicted.
const defaultOperationStoreCapacity = 256

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
	mu       sync.Mutex
	items    map[string]*list.Element
	order    *list.List
	capacity int
}

func newOperationStore() *operationStore {
	return &operationStore{
		items:    make(map[string]*list.Element),
		order:    list.New(),
		capacity: defaultOperationStoreCapacity,
	}
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
	if existing, ok := s.items[id]; ok {
		existing.Value = op
		s.order.MoveToBack(existing)
		return op
	}
	s.items[id] = s.order.PushBack(op)
	s.evictExcess()
	return op
}

func (s *operationStore) get(id string) (operationState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[id]
	if !ok {
		return operationState{}, false
	}
	return el.Value.(operationState), true
}

func (s *operationStore) succeed(id, output string, summary domain.DryRunSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[id]
	if !ok {
		return
	}
	op := el.Value.(operationState)
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
	el.Value = op
	s.evictExcess()
}

func (s *operationStore) fail(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[id]
	if !ok {
		return
	}
	op := el.Value.(operationState)
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
	el.Value = op
	s.evictExcess()
}

func (s *operationStore) progress(id string, progress domain.CleanerProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[id]
	if !ok {
		return
	}
	op := el.Value.(operationState)
	op.Progress = progress
	el.Value = op
}

// evictExcess drops the oldest *finished* operations when the store exceeds
// its capacity. Running operations are preserved so progress polling for
// active jobs is never lost. Caller must hold s.mu.
func (s *operationStore) evictExcess() {
	if s.capacity <= 0 {
		return
	}
	for s.order.Len() > s.capacity {
		victim := s.findEvictable()
		if victim == nil {
			return
		}
		op := victim.Value.(operationState)
		delete(s.items, op.ID)
		s.order.Remove(victim)
	}
}

func (s *operationStore) findEvictable() *list.Element {
	for el := s.order.Front(); el != nil; el = el.Next() {
		if el.Value.(operationState).Status != operationRunning {
			return el
		}
	}
	return nil
}
