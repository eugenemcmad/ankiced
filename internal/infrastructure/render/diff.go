package render

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ankiced/internal/domain"
)

type DiffRenderer struct{}

func (DiffRenderer) Render(records []domain.DiffRecord, summary domain.DryRunSummary, full bool) string {
	var b strings.Builder
	for _, r := range records {
		before := r.Before
		after := r.After
		if !full {
			before = clip(before, 120)
			after = clip(after, 120)
		}
		fmt.Fprintf(&b, "note=%d field=%s\n- %s\n+ %s\n\n", r.NoteID, r.FieldName, before, after)
	}
	fmt.Fprintf(&b, "summary: processed=%d changed=%d skipped=%d errors=%d\n", summary.Processed, summary.Changed, summary.Skipped, summary.Errors)
	return b.String()
}

type JSONReportWriter struct{}

func (JSONReportWriter) WriteReport(_ context.Context, path string, records []domain.DiffRecord, summary domain.DryRunSummary) error {
	payload := map[string]any{
		"records": records,
		"summary": summary,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// clip shortens a string to at most `size` runes (not bytes) so multi-byte
// UTF-8 sequences (Cyrillic, CJK, emoji) are never split mid-character.
func clip(value string, size int) string {
	if size <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= size {
		return value
	}
	return string(runes[:size]) + "..."
}
