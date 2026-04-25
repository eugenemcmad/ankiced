package sqlite

import "ankiced/internal/application"

// Re-export application sentinels so repository errors propagate identity
// across packages and errors.Is matches across layer boundaries.
var (
	ErrDeckNotFound  = application.ErrDeckNotFound
	ErrNoteNotFound  = application.ErrNoteNotFound
	ErrModelNotFound = application.ErrModelNotFound
)
