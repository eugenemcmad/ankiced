package application

import (
	"context"
	"errors"
	"testing"
	"time"

	appconfig "ankiced/internal/config"
	"ankiced/internal/domain"
)

type stubDeckRepo struct {
	nameExists bool
	missing    bool
	renamed    string
	search     string
}

func (s *stubDeckRepo) ListDecks(context.Context) ([]domain.Deck, error) { return nil, nil }
func (s *stubDeckRepo) SearchDecks(_ context.Context, search string) ([]domain.Deck, error) {
	s.search = search
	return []domain.Deck{{ID: 1, Name: "Default", CardCount: 2}}, nil
}
func (s *stubDeckRepo) DeckExists(context.Context, int64) (bool, error) {
	return !s.missing, nil
}
func (s *stubDeckRepo) DeckNameExists(context.Context, string, int64) (bool, error) {
	return s.nameExists, nil
}
func (s *stubDeckRepo) RenameDeck(_ context.Context, _ int64, name string) error {
	s.renamed = name
	return nil
}

type stubNoteRepo struct {
	note     domain.Note
	updated  domain.Note
	noteIDs  []int64
	getErrID int64
}

func (s *stubNoteRepo) ListNotes(context.Context, domain.FilterSet, domain.Pagination) ([]domain.Note, error) {
	return nil, nil
}
func (s *stubNoteRepo) GetNote(_ context.Context, noteID int64) (domain.Note, error) {
	if s.getErrID != 0 && s.getErrID == noteID {
		return domain.Note{}, errors.New("boom")
	}
	return s.note, nil
}
func (s *stubNoteRepo) UpdateNote(_ context.Context, note domain.Note) error {
	s.updated = note
	return nil
}
func (s *stubNoteRepo) ListNoteIDsByDeck(context.Context, int64) ([]int64, error) {
	return s.noteIDs, nil
}

type stubModelRepo struct{ model domain.Model }

func (s stubModelRepo) GetModelByID(context.Context, int64) (domain.Model, error) {
	return s.model, nil
}

type stubConfirm struct {
	ok bool
}

func (s stubConfirm) Confirm(context.Context, string) (bool, error) { return s.ok, nil }

type stubDiff struct{}

func (stubDiff) Render(records []domain.DiffRecord, _ domain.DryRunSummary, _ bool) string {
	if len(records) == 0 {
		return ""
	}
	return "diff"
}

type stubTx struct{}

func (stubTx) WithTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type stubBackupStore struct {
	created int
	dbPath  string
}

func (s *stubBackupStore) CreateBackup(_ context.Context, dbPath string, now time.Time) (domain.BackupInfo, error) {
	s.created++
	s.dbPath = dbPath
	return domain.BackupInfo{Path: dbPath + ".bak", CreatedAt: now}, nil
}

func (s *stubBackupStore) CleanupBackups(context.Context, string, int) error { return nil }

type stubTemplate struct {
	apply func(string) (string, error)
}

func (s stubTemplate) ID() string   { return domain.DefaultActionTemplateID }
func (s stubTemplate) Name() string { return "Stub Template" }
func (s stubTemplate) Apply(value string) (string, error) {
	return s.apply(value)
}

type stubTemplateRegistry struct {
	template domain.ActionTemplate
	gotID    string
}

func (s *stubTemplateRegistry) Get(id string) (domain.ActionTemplate, error) {
	s.gotID = id
	return s.template, nil
}

func (s *stubTemplateRegistry) Default() (domain.ActionTemplate, error) {
	return s.template, nil
}

func templates(fn func(string) string) *stubTemplateRegistry {
	return &stubTemplateRegistry{
		template: stubTemplate{apply: func(value string) (string, error) {
			return fn(value), nil
		}},
	}
}

func TestRenameDeckConflict(t *testing.T) {
	decks := &stubDeckRepo{nameExists: true}
	backups := &stubBackupStore{}
	svc := Services{Decks: decks, Backups: backups}
	err := svc.RenameDeck(context.Background(), 1, "New")
	if !errors.Is(err, domain.ErrDeckNameConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if backups.created != 0 {
		t.Fatalf("expected no backup on conflict, got %+v", backups)
	}
}

func TestRenameDeckMissingDeckDoesNotCreateBackup(t *testing.T) {
	decks := &stubDeckRepo{missing: true}
	backups := &stubBackupStore{}
	svc := Services{Decks: decks, Backups: backups}
	err := svc.RenameDeck(context.Background(), 404, "New")
	if !errors.Is(err, ErrDeckNotFound) {
		t.Fatalf("expected deck not found error, got %v", err)
	}
	if backups.created != 0 {
		t.Fatalf("expected no backup for missing deck, got %+v", backups)
	}
	if decks.renamed != "" {
		t.Fatalf("expected no rename for missing deck, got %q", decks.renamed)
	}
}

func TestRenameDeckCreatesBackupBeforeWrite(t *testing.T) {
	decks := &stubDeckRepo{}
	backups := &stubBackupStore{}
	svc := Services{
		Cfg:     appconfig.Settings{DBPath: "collection.anki2"},
		Decks:   decks,
		Backups: backups,
	}
	err := svc.RenameDeck(context.Background(), 1, "New")
	if err != nil {
		t.Fatalf("rename deck: %v", err)
	}
	if backups.created != 1 || backups.dbPath != "collection.anki2" {
		t.Fatalf("expected backup before rename, got %+v", backups)
	}
}

func TestSearchDecksRejectsEmptyQuery(t *testing.T) {
	svc := Services{Decks: &stubDeckRepo{}}
	_, err := svc.SearchDecks(context.Background(), "   ")
	if !errors.Is(err, domain.ErrDeckSearchEmpty) {
		t.Fatalf("expected ErrDeckSearchEmpty, got %v", err)
	}
}

func TestSearchDecksTrimsQuery(t *testing.T) {
	decks := &stubDeckRepo{}
	svc := Services{Decks: decks}
	_, err := svc.SearchDecks(context.Background(), "  def  ")
	if err != nil {
		t.Fatalf("search decks: %v", err)
	}
	if decks.search != "def" {
		t.Fatalf("expected trimmed search 'def', got %q", decks.search)
	}
}

func TestUpdateNoteSetsModAndUSN(t *testing.T) {
	notes := &stubNoteRepo{note: domain.Note{ID: 10, RawFlds: "a\x1fb", USN: 0}}
	svc := Services{
		Notes:  notes,
		Models: stubModelRepo{model: domain.Model{ID: 0, FieldNames: []string{"Front", "Back"}}},
		Now:    func() time.Time { return time.Unix(123, 0) },
	}
	err := svc.UpdateNote(context.Background(), 10, []domain.NoteField{{Name: "Front", Value: "x"}, {Name: "Back", Value: "y"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notes.updated.USN != -1 || notes.updated.Mod != 123 || notes.updated.RawFlds != "x\x1fy" {
		t.Fatalf("unexpected updated note: %+v", notes.updated)
	}
}

func TestUpdateNoteCreatesBackupBeforeWrite(t *testing.T) {
	notes := &stubNoteRepo{note: domain.Note{ID: 10, ModelID: 5, RawFlds: "a\x1fb", USN: 0}}
	backups := &stubBackupStore{}
	svc := Services{
		Cfg:     appconfig.Settings{DBPath: "collection.anki2"},
		Notes:   notes,
		Models:  stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Backups: backups,
		Now:     func() time.Time { return time.Unix(123, 0) },
	}
	err := svc.UpdateNote(context.Background(), 10, []domain.NoteField{{Name: "Front", Value: "x"}, {Name: "Back", Value: "y"}})
	if err != nil {
		t.Fatalf("update note: %v", err)
	}
	if backups.created != 1 || backups.dbPath != "collection.anki2" {
		t.Fatalf("expected backup before update, got %+v", backups)
	}
}

func TestUpdateNoteRejectsFieldCountMismatch(t *testing.T) {
	notes := &stubNoteRepo{note: domain.Note{ID: 10, ModelID: 5, RawFlds: "a\x1fb", USN: 0}}
	svc := Services{
		Notes:  notes,
		Models: stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Now:    func() time.Time { return time.Unix(123, 0) },
	}
	err := svc.UpdateNote(context.Background(), 10, []domain.NoteField{{Name: "Front", Value: "x"}})
	if !errors.Is(err, domain.ErrFieldCountInvalid) {
		t.Fatalf("expected field count error, got %v", err)
	}
	if notes.updated.RawFlds != "" {
		t.Fatalf("expected no update, got %+v", notes.updated)
	}
}

func TestRunCleanerDryRunSummary(t *testing.T) {
	notes := &stubNoteRepo{
		note:    domain.Note{ID: 1, ModelID: 5, RawFlds: "<u>a</u>\x1fb"},
		noteIDs: []int64{1},
	}
	svc := Services{
		Notes:     notes,
		Models:    stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Confirm:   stubConfirm{ok: true},
		Diff:      stubDiff{},
		Tx:        stubTx{},
		Templates: templates(func(v string) string { return "clean:" + v }),
	}
	out, summary, err := svc.RunCleaner(context.Background(), appconfig.Settings{Workers: 1, ForceApply: true}, 1, true, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" || summary.Processed != 1 || summary.Changed != 1 {
		t.Fatalf("unexpected output/summary: out=%q summary=%+v", out, summary)
	}
}

func TestRunCleanerUsesSelectedTemplateID(t *testing.T) {
	notes := &stubNoteRepo{
		note:    domain.Note{ID: 1, ModelID: 5, RawFlds: "<u>a</u>\x1fb"},
		noteIDs: []int64{1},
	}
	registry := templates(func(v string) string { return "clean:" + v })
	svc := Services{
		Notes:     notes,
		Models:    stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Confirm:   stubConfirm{ok: true},
		Diff:      stubDiff{},
		Tx:        stubTx{},
		Templates: registry,
	}
	_, _, err := svc.RunCleaner(context.Background(), appconfig.Settings{Workers: 1, ForceApply: true}, 1, true, "custom_template", nil)
	if err != nil {
		t.Fatalf("run cleaner: %v", err)
	}
	if registry.gotID != "custom_template" {
		t.Fatalf("expected selected template id, got %q", registry.gotID)
	}
}

func TestRunCleanerReportsProgress(t *testing.T) {
	notes := &stubNoteRepo{
		note:    domain.Note{ID: 1, ModelID: 5, RawFlds: "<u>a</u>\x1fb"},
		noteIDs: []int64{1},
	}
	svc := Services{
		Notes:     notes,
		Models:    stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Confirm:   stubConfirm{ok: true},
		Diff:      stubDiff{},
		Tx:        stubTx{},
		Templates: templates(func(v string) string { return "clean:" + v }),
	}
	var progress []domain.CleanerProgress
	_, _, err := svc.RunCleaner(context.Background(), appconfig.Settings{Workers: 1, ForceApply: true}, 1, true, "", func(p domain.CleanerProgress) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatalf("run cleaner: %v", err)
	}
	if len(progress) == 0 {
		t.Fatal("expected progress events")
	}
	last := progress[len(progress)-1]
	if last.Stage != "finished" || last.Total != 1 || last.Processed != 1 || last.Changed != 1 {
		t.Fatalf("unexpected final progress: %+v", last)
	}
}

func TestRunCleanerDryRunDoesNotCreateBackup(t *testing.T) {
	notes := &stubNoteRepo{
		note:    domain.Note{ID: 1, ModelID: 5, RawFlds: "<u>a</u>\x1fb"},
		noteIDs: []int64{1},
	}
	backups := &stubBackupStore{}
	svc := Services{
		Notes:     notes,
		Models:    stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Confirm:   stubConfirm{ok: true},
		Diff:      stubDiff{},
		Backups:   backups,
		Tx:        stubTx{},
		Templates: templates(func(v string) string { return "clean:" + v }),
	}
	_, _, err := svc.RunCleaner(context.Background(), appconfig.Settings{DBPath: "collection.anki2", Workers: 1, ForceApply: true}, 1, true, "", nil)
	if err != nil {
		t.Fatalf("run cleaner dry run: %v", err)
	}
	if backups.created != 0 {
		t.Fatalf("expected no backup for dry run, got %+v", backups)
	}
}

func TestRunCleanerApplyCreatesSingleBackup(t *testing.T) {
	notes := &stubNoteRepo{
		note:    domain.Note{ID: 1, ModelID: 5, RawFlds: "<u>a</u>\x1fb"},
		noteIDs: []int64{1},
	}
	backups := &stubBackupStore{}
	svc := Services{
		Notes:     notes,
		Models:    stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Confirm:   stubConfirm{ok: true},
		Diff:      stubDiff{},
		Backups:   backups,
		Tx:        stubTx{},
		Templates: templates(func(v string) string { return "clean:" + v }),
		Now:       func() time.Time { return time.Unix(123, 0) },
	}
	_, _, err := svc.RunCleaner(context.Background(), appconfig.Settings{DBPath: "collection.anki2", Workers: 1, ForceApply: true}, 1, false, "", nil)
	if err != nil {
		t.Fatalf("run cleaner apply: %v", err)
	}
	if backups.created != 1 || backups.dbPath != "collection.anki2" {
		t.Fatalf("expected single backup before apply, got %+v", backups)
	}
}

func TestRunCleanerFailFastOnWorkerError(t *testing.T) {
	notes := &stubNoteRepo{
		note:     domain.Note{ID: 1, ModelID: 5, RawFlds: "a\x1fb"},
		noteIDs:  []int64{1, 2},
		getErrID: 2,
	}
	svc := Services{
		Notes:     notes,
		Models:    stubModelRepo{model: domain.Model{ID: 5, FieldNames: []string{"Front", "Back"}}},
		Confirm:   stubConfirm{ok: true},
		Diff:      stubDiff{},
		Tx:        stubTx{},
		Templates: templates(func(v string) string { return v + "!" }),
	}
	_, summary, err := svc.RunCleaner(context.Background(), appconfig.Settings{Workers: 2, ForceApply: true}, 1, true, "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if summary.Errors == 0 {
		t.Fatalf("expected error counter increment, got %+v", summary)
	}
}
