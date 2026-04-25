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

func clip(value string, size int) string {
	if len(value) <= size {
		return value
	}
	return value[:size] + "..."
}
