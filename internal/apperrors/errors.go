package apperrors

import "errors"

type Code string

const (
	CodeOperationCancelled Code = "OPERATION_CANCELLED"
	CodeInvalidEscape      Code = "INVALID_ESCAPE_SEQUENCE"
	CodeDatabasePathEmpty  Code = "DATABASE_PATH_EMPTY"
	CodeDeckNotFound       Code = "DECK_NOT_FOUND"
	CodeNoteNotFound       Code = "NOTE_NOT_FOUND"
	CodeModelNotFound      Code = "MODEL_NOT_FOUND"
	CodeReportWriteFailed  Code = "REPORT_WRITE_FAILED"
	CodeTemplateNotFound   Code = "TEMPLATE_NOT_FOUND"
)

type AppError struct {
	Code    Code
	Message string
	Cause   error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Message != "" && e.Cause != nil:
		return e.Message + ": " + e.Cause.Error()
	case e.Message != "":
		return e.Message
	case e.Cause != nil:
		return e.Cause.Error()
	default:
		return string(e.Code)
	}
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// New constructs an AppError carrying a structured Code and human-readable
// message but no underlying cause. It is the canonical way to declare
// package-level sentinel errors (see internal/application/errors.go) that
// downstream presentation code matches via HasCode / errors.Is.
func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Wrap is the cause-bearing companion to New: it produces a fresh AppError
// with the provided Code and message while preserving the underlying error
// chain via Unwrap. Use it when an operation translates a low-level failure
// (file I/O, parser, ...) into a high-level domain code that the
// presentation layer can map to a user-facing message and a structured API
// error contract. The returned error round-trips through errors.Is /
// errors.As with both the sentinel for `code` and the wrapped cause.
func Wrap(code Code, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

// HasCode reports whether err (or any error in its Unwrap chain) is an
// AppError with the given Code. It is the preferred matcher for translating
// codes into user-facing copy because it survives fmt.Errorf("%w: ...")
// wrapping by callers.
func HasCode(err error, code Code) bool {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Code == code
}
