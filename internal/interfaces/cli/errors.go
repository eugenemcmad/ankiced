package cli

import "ankiced/internal/apperrors"

var (
	ErrInvalidEscapeSequence = apperrors.New(apperrors.CodeInvalidEscape, "invalid escape sequence")
)
