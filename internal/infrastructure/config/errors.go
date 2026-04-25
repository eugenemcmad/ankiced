package config

import "ankiced/internal/apperrors"

var (
	ErrDatabasePathEmpty = apperrors.New(apperrors.CodeDatabasePathEmpty, "database path is empty")
)
