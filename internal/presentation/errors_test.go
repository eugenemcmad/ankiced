package presentation

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"ankiced/internal/application"
	"ankiced/internal/apperrors"
	"ankiced/internal/domain"
)

func TestFormatError_Cases(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"eof", io.EOF, "input stream closed"},
		{"operation_cancelled_sentinel", application.ErrOperationCancelled, "operation cancelled by user"},
		{"operation_cancelled_by_code", apperrors.New(apperrors.CodeOperationCancelled, ""), "operation cancelled by user"},
		{"empty_deck_name", domain.ErrEmptyDeckName, "deck name cannot be empty"},
		{"deck_name_conflict", domain.ErrDeckNameConflict, "deck name is already used"},
		{"deck_name_too_long", domain.ErrDeckNameTooLong, "deck name is too long"},
		{"deck_name_invalid", domain.ErrDeckNameInvalid, "deck name contains control characters"},
		{"deck_search_empty", domain.ErrDeckSearchEmpty, "deck search text cannot be empty"},
		{"invalid_note_filters", domain.ErrInvalidNoteListFilters, "invalid note list query: need a deck id, a note id, or search text for all-decks search"},
		{"invalid_note_id", domain.ErrInvalidNoteID, "note id must be a positive integer"},
		{"field_count_invalid", domain.ErrFieldCountInvalid, "note fields do not match model"},
		{"invalid_escape", apperrors.New(apperrors.CodeInvalidEscape, ""), "invalid escape sequence in multiline input"},
		{"db_path_empty", apperrors.New(apperrors.CodeDatabasePathEmpty, ""), "database path is empty"},
		{"deck_not_found", application.ErrDeckNotFound, "deck not found"},
		{"note_not_found", application.ErrNoteNotFound, "note not found"},
		{"model_not_found", application.ErrModelNotFound, "model not found"},
		{"template_not_found", application.ErrTemplateNotFound, "action template not found"},
		{"report_failed", application.ErrReportWriteFailed, "failed to write report"},
		{"unknown", errors.New("raw error"), "raw error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatError(tc.err)
			if got != tc.want {
				t.Fatalf("FormatError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestFormatDebugError_NoCauseEqualsFormatError(t *testing.T) {
	err := domain.ErrEmptyDeckName
	debug := FormatDebugError(err)
	user := FormatError(err)
	if debug != user {
		t.Fatalf("expected debug==user when no cause, got debug=%q user=%q", debug, user)
	}
}

func TestFormatDebugError_IncludesCauseChain(t *testing.T) {
	wrapped := apperrors.Wrap(apperrors.CodeReportWriteFailed, "failed to write report", fmt.Errorf("disk full: %w", io.ErrShortWrite))
	got := FormatDebugError(wrapped)
	if !strings.Contains(got, "failed to write report") {
		t.Fatalf("expected user message in debug output, got %q", got)
	}
	if !strings.Contains(got, "disk full") {
		t.Fatalf("expected cause chain in debug output, got %q", got)
	}
	if !strings.Contains(got, "cause:") {
		t.Fatalf("expected 'cause:' separator in debug output, got %q", got)
	}
}

func TestFormatError_NilReturnsEmpty(t *testing.T) {
	if got := FormatError(nil); got != "" {
		t.Fatalf("FormatError(nil) = %q, want empty string", got)
	}
	if got := FormatDebugError(nil); got != "" {
		t.Fatalf("FormatDebugError(nil) = %q, want empty string", got)
	}
}
