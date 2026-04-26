package render

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"ankiced/internal/domain"
)

func TestClip_AsciiUnderLimitReturnsAsIs(t *testing.T) {
	if got := clip("hello", 10); got != "hello" {
		t.Fatalf("want %q, got %q", "hello", got)
	}
}

func TestClip_AsciiOverLimitTruncatesAndAppendsEllipsis(t *testing.T) {
	got := clip(strings.Repeat("a", 130), 120)
	want := strings.Repeat("a", 120) + "..."
	if got != want {
		t.Fatalf("want length %d, got %d (%q)", len(want), len(got), got)
	}
}

func TestClip_RuneAwareDoesNotSplitMultibyte(t *testing.T) {
	in := strings.Repeat("я", 200) // 2 bytes per rune
	got := clip(in, 50)
	if !utf8.ValidString(got) {
		t.Fatalf("clipped string is not valid UTF-8: %q", got)
	}
	wantPrefix := strings.Repeat("я", 50)
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("want prefix %q, got %q", wantPrefix, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
}

func TestClip_NonPositiveSizeReturnsEmpty(t *testing.T) {
	if got := clip("anything", 0); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
	if got := clip("anything", -5); got != "" {
		t.Fatalf("want empty for negative, got %q", got)
	}
}

func TestDiffRendererHonoursFullFlag(t *testing.T) {
	records := []domain.DiffRecord{{
		NoteID:    1,
		FieldName: "Front",
		Before:    strings.Repeat("a", 200),
		After:     strings.Repeat("b", 200),
	}}
	summary := domain.DryRunSummary{Processed: 1, Changed: 1}

	clipped := DiffRenderer{}.Render(records, summary, false)
	if !strings.Contains(clipped, strings.Repeat("a", 120)+"...") {
		t.Fatalf("expected before-clip in output, got %q", clipped)
	}
	if strings.Contains(clipped, strings.Repeat("a", 121)) {
		t.Fatalf("clipped output should not contain 121 'a' chars: %q", clipped)
	}

	full := DiffRenderer{}.Render(records, summary, true)
	if !strings.Contains(full, strings.Repeat("a", 200)) {
		t.Fatalf("expected full before in output: %q", full)
	}
	if !strings.Contains(full, "summary: processed=1 changed=1") {
		t.Fatalf("missing summary line: %q", full)
	}
}

func TestDiffRendererWithMultipleRecords(t *testing.T) {
	records := []domain.DiffRecord{
		{NoteID: 1, FieldName: "Front", Before: "a", After: "b"},
		{NoteID: 2, FieldName: "Back", Before: "c", After: "d"},
	}
	out := DiffRenderer{}.Render(records, domain.DryRunSummary{}, true)
	for _, want := range []string{"note=1", "note=2", "field=Front", "field=Back"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestJSONReportWriterWritesValidPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	records := []domain.DiffRecord{{NoteID: 7, FieldName: "F", Before: "x", After: "y"}}
	summary := domain.DryRunSummary{Processed: 1, Changed: 1}

	if err := (JSONReportWriter{}).WriteReport(context.Background(), path, records, summary); err != nil {
		t.Fatalf("write report: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var payload struct {
		Records []domain.DiffRecord  `json:"records"`
		Summary domain.DryRunSummary `json:"summary"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Records) != 1 || payload.Records[0].NoteID != 7 {
		t.Fatalf("unexpected records: %+v", payload.Records)
	}
	if payload.Summary.Processed != 1 || payload.Summary.Changed != 1 {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
}

func TestJSONReportWriterFailsOnInvalidPath(t *testing.T) {
	err := (JSONReportWriter{}).WriteReport(context.Background(), filepath.Join(t.TempDir(), "missing", "deep", "x.json"), nil, domain.DryRunSummary{})
	if err == nil {
		t.Fatalf("expected error writing to non-existent dir")
	}
}
