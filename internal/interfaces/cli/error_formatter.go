package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"ankiced/internal/apperrors"
	"ankiced/internal/domain"
)

// FormatError converts internal errors to user-facing CLI messages.
func FormatError(err error) string {
	switch {
	case errors.Is(err, io.EOF):
		return "input stream closed"
	case apperrors.HasCode(err, apperrors.CodeOperationCancelled):
		return "operation cancelled by user"
	case errors.Is(err, domain.ErrEmptyDeckName):
		return "deck name cannot be empty"
	case errors.Is(err, domain.ErrDeckNameConflict):
		return "deck name is already used"
	case errors.Is(err, domain.ErrDeckNameTooLong):
		return "deck name is too long"
	case errors.Is(err, domain.ErrDeckNameInvalid):
		return "deck name contains control characters"
	case errors.Is(err, domain.ErrDeckSearchEmpty):
		return "deck search text cannot be empty"
	case errors.Is(err, domain.ErrInvalidNoteListFilters):
		return "invalid note list query: need a deck id, a note id, or search text for all-decks search"
	case errors.Is(err, domain.ErrInvalidNoteID):
		return "note id must be a positive integer"
	case errors.Is(err, domain.ErrFieldCountInvalid):
		return "note fields do not match model"
	case apperrors.HasCode(err, apperrors.CodeInvalidEscape):
		return "invalid escape sequence in multiline input"
	case apperrors.HasCode(err, apperrors.CodeDatabasePathEmpty):
		return "database path is empty"
	case apperrors.HasCode(err, apperrors.CodeDeckNotFound):
		return "deck not found"
	case apperrors.HasCode(err, apperrors.CodeModelNotFound):
		return "model not found"
	case apperrors.HasCode(err, apperrors.CodeReportWriteFailed):
		return "failed to write report"
	default:
		return err.Error()
	}
}

// FormatDebugError returns user-facing message plus unwrap chain.
func FormatDebugError(err error) string {
	base := FormatError(err)
	chain := unwrapChain(err)
	if len(chain) == 0 {
		return base
	}
	return fmt.Sprintf("%s | cause: %s", base, strings.Join(chain, " -> "))
}

func unwrapChain(err error) []string {
	chain := make([]string, 0)
	for current := errors.Unwrap(err); current != nil; current = errors.Unwrap(current) {
		chain = append(chain, current.Error())
	}
	return chain
}
