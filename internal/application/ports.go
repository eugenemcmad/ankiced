package application

import (
	"context"
	"time"

	"ankiced/internal/domain"
)

type LogEvent struct {
	ID            int64          `json:"id"`
	Timestamp     string         `json:"ts"`
	Level         string         `json:"level"`
	Operation     string         `json:"operation"`
	Message       string         `json:"message"`
	Details       map[string]any `json:"details,omitempty"`
	CorrelationID string         `json:"correlation_id"`
}

type LogBroadcaster interface {
	Publish(event LogEvent)
	Since(id int64) []LogEvent
	Subscribe() (<-chan LogEvent, func())
}

type BackupStore interface {
	CreateBackup(ctx context.Context, dbPath string, now time.Time) (domain.BackupInfo, error)
	CleanupBackups(ctx context.Context, dbPath string, keepLastN int) error
}

type ConfirmPrompter interface {
	Confirm(ctx context.Context, prompt string) (bool, error)
}

type DeckRepository interface {
	ListDecks(ctx context.Context) ([]domain.Deck, error)
	SearchDecks(ctx context.Context, search string) ([]domain.Deck, error)
	DeckExists(ctx context.Context, id int64) (bool, error)
	DeckNameExists(ctx context.Context, name string, excludeID int64) (bool, error)
	RenameDeck(ctx context.Context, id int64, name string) error
}

type NoteRepository interface {
	ListNotes(ctx context.Context, filters domain.FilterSet, page domain.Pagination) ([]domain.Note, error)
	GetNote(ctx context.Context, noteID int64) (domain.Note, error)
	UpdateNote(ctx context.Context, note domain.Note) error
	ListNoteIDsByDeck(ctx context.Context, deckID int64) ([]int64, error)
}

type ModelRepository interface {
	GetModelByID(ctx context.Context, modelID int64) (domain.Model, error)
}

type TemplateRegistry interface {
	Get(id string) (domain.ActionTemplate, error)
	Default() (domain.ActionTemplate, error)
}

type TransactionManager interface {
	WithTx(ctx context.Context, fn func(context.Context) error) error
}

type DiffRenderer interface {
	Render(records []domain.DiffRecord, summary domain.DryRunSummary, full bool) string
}

type ReportWriter interface {
	WriteReport(ctx context.Context, path string, records []domain.DiffRecord, summary domain.DryRunSummary) error
}
