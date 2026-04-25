package integration

import (
	"context"
	"errors"
	"testing"

	"ankiced/internal/application"
	appconfig "ankiced/internal/config"
	"ankiced/internal/domain"
	sqliteinfra "ankiced/internal/infrastructure/sqlite"
)

type alwaysConfirm struct{}

func (alwaysConfirm) Confirm(context.Context, string) (bool, error) { return true, nil }

func TestDeckAndNoteFlows(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	ctx := context.Background()

	svc := setupTestServices(db, alwaysConfirm{})

	list, err := svc.ListDecks(ctx)
	if err != nil {
		t.Fatalf("list decks: %v", err)
	}
	if len(list) != 1 || list[0].CardCount != 2 {
		t.Fatalf("unexpected decks: %+v", list)
	}

	if err := svc.RenameDeck(ctx, 1, "Renamed"); err != nil {
		t.Fatalf("rename deck: %v", err)
	}
	if err := svc.RenameDeck(ctx, 1, "bad\tname"); !errors.Is(err, domain.ErrDeckNameInvalid) {
		t.Fatalf("expected invalid deck name, got %v", err)
	}

	note, err := svc.GetNote(ctx, 100)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if len(note.Fields) != 2 || note.Fields[0].Name != "Front" {
		t.Fatalf("unexpected fields: %+v", note.Fields)
	}

	if err := svc.UpdateNote(ctx, 101, []domain.NoteField{{Name: "Front", Value: "A"}, {Name: "Back", Value: "B"}}); err != nil {
		t.Fatalf("update note: %v", err)
	}

	filtered, err := svc.ListNotes(ctx, domain.FilterSet{
		DeckID:      1,
		ModFromUnix: 2,
		ModToUnix:   10,
	}, domain.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("list notes with mod range: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("expected empty list for mod range filter, got %+v", filtered)
	}

	if _, err := db.SQL.Exec(`INSERT INTO notes(id,guid,mid,flds,mod,usn) VALUES (102,'g3',10,'orphanonly',42,0)`); err != nil {
		t.Fatalf("insert orphan note: %v", err)
	}

	byID, err := svc.ListNotes(ctx, domain.FilterSet{NoteID: 102}, domain.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("list by note id: %v", err)
	}
	if len(byID) != 1 || byID[0].ID != 102 {
		t.Fatalf("expected note 102 by id, got %+v", byID)
	}

	missing, err := svc.ListNotes(ctx, domain.FilterSet{NoteID: 9999}, domain.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("list missing note id: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected empty for unknown id, got %+v", missing)
	}

	global, err := svc.ListNotes(ctx, domain.FilterSet{DeckID: 0, SearchText: "orphan"}, domain.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("global search: %v", err)
	}
	if len(global) != 1 || global[0].ID != 102 {
		t.Fatalf("expected global hit for orphan note, got %+v", global)
	}

	deckText, err := svc.ListNotes(ctx, domain.FilterSet{DeckID: 1, SearchText: "hello"}, domain.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("deck-scoped text search: %v", err)
	}
	if len(deckText) != 1 || deckText[0].ID != 100 {
		t.Fatalf("expected deck note 100 for hello, got %+v", deckText)
	}

	if _, err := svc.ListNotes(ctx, domain.FilterSet{DeckID: 0, SearchText: ""}, domain.Pagination{Limit: 10, Offset: 0}); !errors.Is(err, domain.ErrInvalidNoteListFilters) {
		t.Fatalf("expected ErrInvalidNoteListFilters for empty global query, got %v", err)
	}
}

func TestDryRunAndApplyCleaner(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	ctx := context.Background()

	svc := setupTestServices(db, alwaysConfirm{})

	out, summary, err := svc.RunCleaner(ctx, appconfig.Settings{Workers: 2, FullDiff: true, ForceApply: true}, 1, true, "", nil)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if summary.Changed == 0 || out == "" {
		t.Fatalf("expected dry run output, got summary=%+v", summary)
	}
	if _, _, err := svc.RunCleaner(ctx, appconfig.Settings{Workers: 2, ForceApply: true}, 1, false, "", nil); err != nil {
		t.Fatalf("apply cleaner: %v", err)
	}
}

func TestListDecksIncludesZeroCardDeckAndStableOrder(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	ctx := context.Background()

	if _, err := db.SQL.Exec(`INSERT INTO decks(id, name) VALUES (2, 'Two')`); err != nil {
		t.Fatalf("insert second deck: %v", err)
	}
	if _, err := db.SQL.Exec(`DELETE FROM cards WHERE did = 2`); err != nil {
		t.Fatalf("cleanup cards: %v", err)
	}
	svc := application.Services{
		Decks: sqliteinfra.NewDeckRepo(db),
	}
	decks, err := svc.ListDecks(ctx)
	if err != nil {
		t.Fatalf("list decks: %v", err)
	}
	if len(decks) != 2 {
		t.Fatalf("expected 2 decks, got %+v", decks)
	}
	if decks[0].ID != 1 || decks[1].ID != 2 {
		t.Fatalf("expected deterministic order by id, got %+v", decks)
	}
	if decks[1].CardCount != 0 {
		t.Fatalf("expected zero card count for deck 2, got %+v", decks[1])
	}
}

func TestSearchDecksByText(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	ctx := context.Background()

	if _, err := db.SQL.Exec(`INSERT INTO decks(id, name) VALUES (2, 'Grammar Deck')`); err != nil {
		t.Fatalf("insert grammar deck: %v", err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO decks(id, name) VALUES (3, 'History')`); err != nil {
		t.Fatalf("insert history deck: %v", err)
	}

	svc := application.Services{
		Decks: sqliteinfra.NewDeckRepo(db),
	}
	decks, err := svc.SearchDecks(ctx, "Deck")
	if err != nil {
		t.Fatalf("search decks: %v", err)
	}
	if len(decks) != 1 || decks[0].Name != "Grammar Deck" {
		t.Fatalf("expected one deck by search, got %+v", decks)
	}
}

func TestGetNoteUsesFieldsTableMetadata(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	ctx := context.Background()

	if _, err := db.SQL.Exec(`UPDATE fields SET name = 'FrontCustom' WHERE ntid = 10 AND ord = 0`); err != nil {
		t.Fatalf("update fields metadata front: %v", err)
	}
	if _, err := db.SQL.Exec(`UPDATE fields SET name = 'BackCustom' WHERE ntid = 10 AND ord = 1`); err != nil {
		t.Fatalf("update fields metadata back: %v", err)
	}

	svc := setupTestServices(db, alwaysConfirm{})
	note, err := svc.GetNote(ctx, 100)
	if err != nil {
		t.Fatalf("get note with models table fallback: %v", err)
	}
	if len(note.Fields) != 2 || note.Fields[0].Name != "FrontCustom" || note.Fields[1].Name != "BackCustom" {
		t.Fatalf("unexpected mapped fields from fields table: %+v", note.Fields)
	}
}

func TestGetNoteFallsBackToColModelsWhenModelsTableMissingModel(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	ctx := context.Background()

	if _, err := db.SQL.Exec(`DELETE FROM models WHERE id = 10`); err != nil {
		t.Fatalf("delete models row: %v", err)
	}
	if _, err := db.SQL.Exec(`DELETE FROM fields WHERE ntid = 10`); err != nil {
		t.Fatalf("delete fields rows: %v", err)
	}
	if _, err := db.SQL.Exec(`CREATE TABLE col (models TEXT NOT NULL)`); err != nil {
		t.Fatalf("create col table: %v", err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO col(models) VALUES ('{"10":{"flds":[{"name":"FrontFromCol"},{"name":"BackFromCol"}]}}')`); err != nil {
		t.Fatalf("seed col.models: %v", err)
	}

	svc := setupTestServices(db, alwaysConfirm{})
	note, err := svc.GetNote(ctx, 100)
	if err != nil {
		t.Fatalf("get note with col.models fallback: %v", err)
	}
	if len(note.Fields) != 2 || note.Fields[0].Name != "FrontFromCol" || note.Fields[1].Name != "BackFromCol" {
		t.Fatalf("unexpected mapped fields from col.models fallback: %+v", note.Fields)
	}
}
