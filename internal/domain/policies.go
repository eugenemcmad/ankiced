package domain

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrEmptyDeckName     = errors.New("deck name is empty")
	ErrDeckNameConflict  = errors.New("deck name conflict")
	ErrFieldCountInvalid = errors.New("note field count does not match model")
	ErrDeckNameTooLong   = errors.New("deck name is too long")
	ErrDeckNameInvalid   = errors.New("deck name contains invalid characters")
	ErrDeckSearchEmpty   = errors.New("deck search text cannot be empty")
	// ErrInvalidNoteListFilters is returned when ListNotes is called without a valid
	// deck scope, note id lookup, or collection-wide search text.
	ErrInvalidNoteListFilters = errors.New("specify a positive deck id, a positive note id, or non-empty search text for collection-wide search")
	ErrInvalidNoteID          = errors.New("note id must be a positive integer")
)

const maxDeckNameLength = 200

func ValidateDeckRename(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ErrEmptyDeckName
	}
	if len([]rune(trimmed)) > maxDeckNameLength {
		return ErrDeckNameTooLong
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return ErrDeckNameInvalid
		}
	}
	return nil
}

func MapFields(fieldValues []string, model Model) ([]NoteField, error) {
	if len(fieldValues) != len(model.FieldNames) {
		return nil, ErrFieldCountInvalid
	}
	result := make([]NoteField, 0, len(fieldValues))
	for i := range fieldValues {
		result = append(result, NoteField{Name: model.FieldNames[i], Value: fieldValues[i]})
	}
	return result, nil
}

func JoinFieldValues(fields []NoteField) []string {
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		result = append(result, f.Value)
	}
	return result
}
