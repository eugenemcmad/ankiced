package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"ankiced/internal/apperrors"
	"ankiced/internal/application"
	appconfig "ankiced/internal/config"
	"ankiced/internal/domain"
	"ankiced/internal/presentation"
)

type captureNoteRepo struct {
	filters domain.FilterSet
	page    domain.Pagination
}

func (c *captureNoteRepo) ListNotes(_ context.Context, filters domain.FilterSet, page domain.Pagination) ([]domain.Note, error) {
	c.filters = filters
	c.page = page
	return []domain.Note{{ID: 1, Mod: 10, RawFlds: "<b>q</b>\x1f<div>a</div>"}}, nil
}

func (c *captureNoteRepo) GetNote(context.Context, int64) (domain.Note, error) {
	return domain.Note{}, nil
}

func (c *captureNoteRepo) UpdateNote(context.Context, domain.Note) error {
	return nil
}

func (c *captureNoteRepo) ListNoteIDsByDeck(context.Context, int64) ([]int64, error) {
	return nil, nil
}

type noopDeckRepo struct{}

func (noopDeckRepo) ListDecks(context.Context) ([]domain.Deck, error) { return nil, nil }
func (noopDeckRepo) SearchDecks(_ context.Context, search string) ([]domain.Deck, error) {
	if strings.TrimSpace(search) == "" {
		return nil, domain.ErrDeckSearchEmpty
	}
	return []domain.Deck{{ID: 2, Name: "My Deck", CardCount: 3}}, nil
}
func (noopDeckRepo) DeckNameExists(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (noopDeckRepo) DeckExists(context.Context, int64) (bool, error) { return true, nil }
func (noopDeckRepo) RenameDeck(context.Context, int64, string) error { return nil }

type noopModelRepo struct{}

func (noopModelRepo) GetModelByID(context.Context, int64) (domain.Model, error) {
	return domain.Model{}, nil
}

func TestListNotesUsesDefaultsAndFilters(t *testing.T) {
	noteRepo := &captureNoteRepo{}
	app := App{
		Svc: application.Services{
			Decks:  noopDeckRepo{},
			Notes:  noteRepo,
			Models: noopModelRepo{},
		},
		Cfg: appconfig.Settings{DefaultPageSize: 25},
		In:  strings.NewReader("3\n1\n\n5\n10\n\nhello\n0\n"),
		Out: &bytes.Buffer{},
	}

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if noteRepo.page.Limit != 25 {
		t.Fatalf("expected default page size 25, got %d", noteRepo.page.Limit)
	}
	if noteRepo.page.Offset != 5 {
		t.Fatalf("expected offset 5, got %d", noteRepo.page.Offset)
	}
	if noteRepo.filters.ModFromUnix != 10 {
		t.Fatalf("expected mod from 10, got %d", noteRepo.filters.ModFromUnix)
	}
	if noteRepo.filters.ModToUnix != 0 {
		t.Fatalf("expected mod to 0, got %d", noteRepo.filters.ModToUnix)
	}
	if noteRepo.filters.SearchText != "hello" {
		t.Fatalf("expected search text hello, got %q", noteRepo.filters.SearchText)
	}
}

func TestReadMultilineDecodesEscapes(t *testing.T) {
	value, err := readMultiline(bufio.NewReader(strings.NewReader("line\\nnext\n.end\n")))
	if err != nil {
		t.Fatalf("read multiline: %v", err)
	}
	if value != "line\nnext" {
		t.Fatalf("unexpected value %q", value)
	}
}

func TestPreviewTextStripsHTML(t *testing.T) {
	got := previewText("<b>hello</b>\x1f<div>world</div>")
	if got != "hello | world" {
		t.Fatalf("unexpected preview %q", got)
	}
}

func TestFindNoteByIDPassesNoteIDFilter(t *testing.T) {
	noteRepo := &captureNoteRepo{}
	app := App{
		Svc: application.Services{
			Decks:  noopDeckRepo{},
			Notes:  noteRepo,
			Models: noopModelRepo{},
		},
		Cfg: appconfig.Settings{DefaultPageSize: 10},
		In:  strings.NewReader("8\n55\n0\n"),
		Out: &bytes.Buffer{},
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run app: %v", err)
	}
	if noteRepo.filters.NoteID != 55 {
		t.Fatalf("expected note id filter 55, got %d", noteRepo.filters.NoteID)
	}
	if noteRepo.filters.DeckID != 0 {
		t.Fatalf("expected deck id unset, got %d", noteRepo.filters.DeckID)
	}
}

func TestSearchDecksMenuOptionWritesDecks(t *testing.T) {
	app := App{
		Svc: application.Services{
			Decks:  noopDeckRepo{},
			Notes:  &captureNoteRepo{},
			Models: noopModelRepo{},
		},
		Cfg: appconfig.Settings{DefaultPageSize: 10},
		In:  strings.NewReader("10\ndeck\n0\n"),
		Out: &bytes.Buffer{},
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run app: %v", err)
	}
	got := app.Out.(*bytes.Buffer).String()
	if !strings.Contains(got, "id=2 name=My Deck cards=3") {
		t.Fatalf("expected searched deck in output, got %q", got)
	}
}

func TestFormatErrorInvalidNoteListFilters(t *testing.T) {
	got := presentation.FormatError(domain.ErrInvalidNoteListFilters)
	if got == "" || got == domain.ErrInvalidNoteListFilters.Error() {
		t.Fatalf("expected friendly message, got %q", got)
	}
}

func TestFormatErrorInvalidNoteID(t *testing.T) {
	got := presentation.FormatError(domain.ErrInvalidNoteID)
	if got != "note id must be a positive integer" {
		t.Fatalf("unexpected message %q", got)
	}
}

func TestFormatErrorFromAppErrorCode(t *testing.T) {
	err := fmt.Errorf("runtime: %w", apperrors.New(apperrors.CodeDeckNotFound, "deck not found"))
	got := presentation.FormatError(err)
	if got != "deck not found" {
		t.Fatalf("unexpected friendly error %q", got)
	}
}

func TestFormatDebugErrorIncludesCauseChain(t *testing.T) {
	err := fmt.Errorf("runtime: %w", apperrors.Wrap(apperrors.CodeInvalidEscape, "invalid escape sequence", fmt.Errorf("invalid syntax")))
	got := presentation.FormatDebugError(err)
	if !strings.Contains(got, "invalid escape sequence in multiline input") {
		t.Fatalf("expected user-facing message, got %q", got)
	}
	if !strings.Contains(got, "cause:") {
		t.Fatalf("expected cause chain, got %q", got)
	}
}
