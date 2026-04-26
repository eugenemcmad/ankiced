package httpapi

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ankiced/internal/application"
)

type logHub struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]chan application.LogEvent
	buf    []application.LogEvent
}

func (h *apiHandler) correlationID() string {
	n := atomic.AddUint64(&h.seq, 1)
	return fmt.Sprintf("req-%d-%d", time.Now().UnixMilli(), n)
}

func (h *apiHandler) log(level, operation, cid, message string, details map[string]any) {
	id := atomic.AddUint64(&h.logSeq, 1)
	entry := application.LogEvent{
		ID:            int64(id),
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Level:         level,
		Operation:     operation,
		Message:       message,
		Details:       details,
		CorrelationID: cid,
	}
	h.logs.Publish(entry)
	if h.logger == nil {
		return
	}
	args := []any{"operation", operation, "correlation_id", cid, "details", details}
	switch strings.ToLower(level) {
	case "error":
		h.logger.Error(message, args...)
	case "warn", "warning":
		h.logger.Warn(message, args...)
	case "debug":
		h.logger.Debug(message, args...)
	default:
		h.logger.Info(message, args...)
	}
}

func newLogHub() *logHub {
	return &logHub{subs: make(map[int]chan application.LogEvent)}
}

func (h *logHub) Subscribe() (<-chan application.LogEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan application.LogEvent, 64)
	id := h.nextID
	h.nextID++
	h.subs[id] = ch
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if existing, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(existing)
		}
	}
}

func (h *logHub) Publish(e application.LogEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = append(h.buf, e)
	if len(h.buf) > 1000 {
		h.buf = h.buf[len(h.buf)-1000:]
	}
	for _, sub := range h.subs {
		select {
		case sub <- e:
		default:
		}
	}
}

func (h *logHub) Since(id int64) []application.LogEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.buf) == 0 {
		return nil
	}
	result := make([]application.LogEvent, 0, len(h.buf))
	for _, item := range h.buf {
		if item.ID > id {
			result = append(result, item)
		}
	}
	return result
}
