package application

import "ankiced/internal/apperrors"

var (
	ErrOperationCancelled = apperrors.New(apperrors.CodeOperationCancelled, "operation cancelled")
	ErrReportWriteFailed  = apperrors.New(apperrors.CodeReportWriteFailed, "failed to write report")
	ErrTemplateNotFound   = apperrors.New(apperrors.CodeTemplateNotFound, "action template not found")
	ErrDeckNotFound       = apperrors.New(apperrors.CodeDeckNotFound, "deck not found")
	ErrNoteNotFound       = apperrors.New(apperrors.CodeNoteNotFound, "note not found")
	ErrModelNotFound      = apperrors.New(apperrors.CodeModelNotFound, "model not found")
)
