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

func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code Code, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

func HasCode(err error, code Code) bool {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Code == code
}
