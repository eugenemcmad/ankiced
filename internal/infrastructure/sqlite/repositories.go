package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ankiced/internal/domain"
)

type DeckRepo struct{ db *DB }
type NoteRepo struct{ db *DB }
type ModelRepo struct{ db *DB }

// likeEscapeChar is the explicit escape character used together with
// SQLite's LIKE operator when a user-supplied substring may contain '%' or
// '_' literals that would otherwise act as wildcards.
const likeEscapeChar = `\`

// escapeLike returns s with the LIKE wildcard characters '%' and '_' (and
// the escape character itself) prefixed by likeEscapeChar so the value is
// matched literally when used inside a LIKE pattern. The caller must pair
// this with `LIKE ? ESCAPE '\'` in the SQL statement.
func escapeLike(s string) string {
	if s == "" {
		return s
	}
	r := strings.NewReplacer(
		likeEscapeChar, likeEscapeChar+likeEscapeChar,
		"%", likeEscapeChar+"%",
		"_", likeEscapeChar+"_",
	)
	return r.Replace(s)
}

func NewDeckRepo(db *DB) DeckRepo { return DeckRepo{db: db} }
func NewNoteRepo(db *DB) NoteRepo { return NoteRepo{db: db} }
func NewModelRepo(db *DB) ModelRepo {
	return ModelRepo{db: db}
}

func (r DeckRepo) ListDecks(ctx context.Context) ([]domain.Deck, error) {
	q := queryer(ctx, r.db)
	rows, err := q.QueryContext(ctx, `
SELECT d.id, d.name, COALESCE(c.card_count, 0)
FROM decks d
LEFT JOIN (
	SELECT did, COUNT(*) AS card_count
	FROM cards
	GROUP BY did
) c ON c.did = d.id
ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Deck, 0)
	for rows.Next() {
		var d domain.Deck
		if err := rows.Scan(&d.ID, &d.Name, &d.CardCount); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r DeckRepo) SearchDecks(ctx context.Context, search string) ([]domain.Deck, error) {
	q := queryer(ctx, r.db)
	rows, err := q.QueryContext(ctx, `
SELECT d.id, d.name, COALESCE(c.card_count, 0)
FROM decks d
LEFT JOIN (
	SELECT did, COUNT(*) AS card_count
	FROM cards
	GROUP BY did
) c ON c.did = d.id
WHERE d.name LIKE ? ESCAPE '\' COLLATE NOCASE
ORDER BY d.id`, "%"+escapeLike(strings.TrimSpace(search))+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Deck, 0)
	for rows.Next() {
		var d domain.Deck
		if err := rows.Scan(&d.ID, &d.Name, &d.CardCount); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r DeckRepo) DeckNameExists(ctx context.Context, name string, excludeID int64) (bool, error) {
	q := queryer(ctx, r.db)
	row := q.QueryRowContext(ctx,
		`SELECT 1 FROM decks WHERE id != ? AND name = ? COLLATE NOCASE LIMIT 1`,
		excludeID, strings.TrimSpace(name))
	var found int
	if err := row.Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r DeckRepo) DeckExists(ctx context.Context, id int64) (bool, error) {
	q := queryer(ctx, r.db)
	row := q.QueryRowContext(ctx, "SELECT 1 FROM decks WHERE id = ? LIMIT 1", id)
	var found int
	if err := row.Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r DeckRepo) RenameDeck(ctx context.Context, id int64, name string) error {
	q := queryer(ctx, r.db)
	res, err := q.ExecContext(ctx, "UPDATE decks SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w: %d", ErrDeckNotFound, id)
	}
	return nil
}

// notesQueryShape is the SELECT projection used by note list queries; the
// CountNotes counterpart wraps a SELECT 1 form of the same FROM/WHERE so
// pagination math stays consistent with the underlying filters.
const notesQueryShape = `SELECT DISTINCT n.id, n.guid, n.mid, n.flds, n.mod, n.usn`

// buildNotesQuery returns the FROM/WHERE fragment and bound args for the
// shared note-listing SQL. The caller is expected to prepend a SELECT
// projection and append ORDER BY / LIMIT / OFFSET as appropriate.
func buildNotesQuery(filters domain.FilterSet) (string, []any, error) {
	// Global text search across notes.flds (no deck filter).
	if filters.DeckID == 0 && strings.TrimSpace(filters.SearchText) != "" {
		fromWhere := ` FROM notes n WHERE n.flds LIKE ? ESCAPE '\' COLLATE NOCASE`
		args := []any{"%" + escapeLike(filters.SearchText) + "%"}
		if filters.ModFromUnix > 0 {
			fromWhere += " AND n.mod >= ?"
			args = append(args, filters.ModFromUnix)
		}
		if filters.ModToUnix > 0 {
			fromWhere += " AND n.mod <= ?"
			args = append(args, filters.ModToUnix)
		}
		return fromWhere, args, nil
	}

	if filters.DeckID <= 0 {
		return "", nil, domain.ErrInvalidNoteListFilters
	}

	fromWhere := `
FROM notes n
JOIN cards c ON c.nid = n.id
WHERE c.did = ?`
	args := []any{filters.DeckID}
	if filters.ModFromUnix > 0 {
		fromWhere += " AND n.mod >= ?"
		args = append(args, filters.ModFromUnix)
	}
	if filters.ModToUnix > 0 {
		fromWhere += " AND n.mod <= ?"
		args = append(args, filters.ModToUnix)
	}
	if filters.SearchText != "" {
		fromWhere += ` AND n.flds LIKE ? ESCAPE '\' COLLATE NOCASE`
		args = append(args, "%"+escapeLike(filters.SearchText)+"%")
	}
	return fromWhere, args, nil
}

func (r NoteRepo) ListNotes(ctx context.Context, filters domain.FilterSet, page domain.Pagination) (result []domain.Note, err error) {
	if page.Limit <= 0 {
		page.Limit = 10
	}
	q := queryer(ctx, r.db)

	// 3.6: exact note id lookup (no deck join, no pagination).
	if filters.NoteID > 0 {
		row := q.QueryRowContext(ctx, `SELECT id, guid, mid, flds, mod, usn FROM notes WHERE id = ?`, filters.NoteID)
		var n domain.Note
		scanErr := row.Scan(&n.ID, &n.GUID, &n.ModelID, &n.RawFlds, &n.Mod, &n.USN)
		if errors.Is(scanErr, sql.ErrNoRows) {
			return []domain.Note{}, nil
		}
		if scanErr != nil {
			return nil, scanErr
		}
		return []domain.Note{n}, nil
	}

	fromWhere, args, err := buildNotesQuery(filters)
	if err != nil {
		return nil, err
	}
	base := notesQueryShape + fromWhere + " ORDER BY n.id LIMIT ? OFFSET ?"
	args = append(args, page.Limit, page.Offset)
	return r.queryNotesRows(ctx, q, base, args)
}

// CountNotes returns the total number of notes matching the same filters as
// ListNotes (sans pagination) so HTTP callers can render accurate page
// counters. NoteID lookups always resolve to 0 or 1 without touching the
// deck join.
func (r NoteRepo) CountNotes(ctx context.Context, filters domain.FilterSet) (int64, error) {
	q := queryer(ctx, r.db)
	if filters.NoteID > 0 {
		row := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes WHERE id = ?`, filters.NoteID)
		var c int64
		if err := row.Scan(&c); err != nil {
			return 0, err
		}
		return c, nil
	}
	fromWhere, args, err := buildNotesQuery(filters)
	if err != nil {
		return 0, err
	}
	// Wrap in a subquery so the DISTINCT semantics in the projection
	// translate into an accurate count even when a note appears in
	// multiple cards within the same deck.
	query := `SELECT COUNT(*) FROM (` + notesQueryShape + fromWhere + `)`
	row := q.QueryRowContext(ctx, query, args...)
	var c int64
	if err := row.Scan(&c); err != nil {
		return 0, err
	}
	return c, nil
}

func (r NoteRepo) queryNotesRows(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, base string, args []any) (result []domain.Note, err error) {
	rows, err := q.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close notes rows: %w", closeErr))
		}
	}()
	result = make([]domain.Note, 0)
	for rows.Next() {
		var n domain.Note
		if scanErr := rows.Scan(&n.ID, &n.GUID, &n.ModelID, &n.RawFlds, &n.Mod, &n.USN); scanErr != nil {
			return nil, scanErr
		}
		result = append(result, n)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return result, nil
}

func (r NoteRepo) GetNote(ctx context.Context, noteID int64) (domain.Note, error) {
	q := queryer(ctx, r.db)
	row := q.QueryRowContext(ctx, "SELECT id, guid, mid, flds, mod, usn FROM notes WHERE id = ?", noteID)
	var n domain.Note
	err := row.Scan(&n.ID, &n.GUID, &n.ModelID, &n.RawFlds, &n.Mod, &n.USN)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Note{}, fmt.Errorf("%w: %d", ErrNoteNotFound, noteID)
		}
		return domain.Note{}, err
	}
	return n, nil
}

func (r NoteRepo) UpdateNote(ctx context.Context, note domain.Note) error {
	q := queryer(ctx, r.db)
	res, err := q.ExecContext(ctx, "UPDATE notes SET flds = ?, mod = ?, usn = ? WHERE id = ?", note.RawFlds, note.Mod, note.USN, note.ID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w: %d", ErrNoteNotFound, note.ID)
	}
	return nil
}

func (r NoteRepo) ListNoteIDsByDeck(ctx context.Context, deckID int64) (ids []int64, err error) {
	q := queryer(ctx, r.db)
	rows, err := q.QueryContext(ctx, "SELECT DISTINCT nid FROM cards WHERE did = ? ORDER BY nid", deckID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close note ids rows: %w", closeErr))
		}
	}()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}
	return ids, nil
}

// GetModelByID resolves a note model from the modern schema (`fields` table),
// then falls back to legacy locations (`models` table, `col.models` JSON).
// Errors from each attempt are joined so callers retain full diagnostic
// context. ErrModelNotFound is preserved through wrapping for errors.Is.
func (r ModelRepo) GetModelByID(ctx context.Context, modelID int64) (domain.Model, error) {
	q := queryer(ctx, r.db)
	model, fieldsErr := r.getModelFromFieldsTable(ctx, q, modelID)
	if fieldsErr == nil {
		return model, nil
	}
	model, modelsErr := r.getModelFromModelsTable(ctx, q, modelID)
	if modelsErr == nil {
		return model, nil
	}
	model, colErr := r.getModelFromCol(ctx, q, modelID)
	if colErr == nil {
		return model, nil
	}
	combined := errors.Join(fieldsErr, modelsErr, colErr)
	if errors.Is(combined, ErrModelNotFound) {
		return domain.Model{}, fmt.Errorf("%w: %d", ErrModelNotFound, modelID)
	}
	return domain.Model{}, combined
}

func (r ModelRepo) getModelFromFieldsTable(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, modelID int64) (model domain.Model, err error) {
	rows, err := q.QueryContext(ctx, "SELECT name FROM fields WHERE ntid = ? ORDER BY ord", modelID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return domain.Model{}, ErrModelNotFound
		}
		return domain.Model{}, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	fieldNames := make([]string, 0, 8)
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return domain.Model{}, scanErr
		}
		fieldNames = append(fieldNames, name)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return domain.Model{}, rowsErr
	}
	if len(fieldNames) == 0 {
		return domain.Model{}, ErrModelNotFound
	}
	return domain.Model{ID: modelID, FieldNames: fieldNames}, nil
}

func (r ModelRepo) getModelFromModelsTable(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, modelID int64) (domain.Model, error) {
	row := q.QueryRowContext(ctx, "SELECT flds FROM models WHERE id = ?", modelID)
	var fieldsJSON string
	if err := row.Scan(&fieldsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Model{}, ErrModelNotFound
		}
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return domain.Model{}, ErrModelNotFound
		}
		return domain.Model{}, err
	}
	fields, err := parseFieldNamesFromFieldsArray(fieldsJSON)
	if err != nil {
		return domain.Model{}, fmt.Errorf("invalid models.flds JSON for model %d: %w", modelID, err)
	}
	return domain.Model{ID: modelID, FieldNames: fields}, nil
}

func (r ModelRepo) getModelFromCol(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, modelID int64) (domain.Model, error) {
	row := q.QueryRowContext(ctx, "SELECT models FROM col LIMIT 1")
	var modelsJSON string
	if err := row.Scan(&modelsJSON); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return domain.Model{}, ErrModelNotFound
		}
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Model{}, ErrModelNotFound
		}
		return domain.Model{}, err
	}
	if strings.TrimSpace(modelsJSON) == "" {
		return domain.Model{}, ErrModelNotFound
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(modelsJSON), &raw); err != nil {
		return domain.Model{}, fmt.Errorf("invalid col.models JSON: %w", err)
	}
	modelMeta, ok := raw[fmt.Sprintf("%d", modelID)]
	if !ok {
		return domain.Model{}, ErrModelNotFound
	}
	var node struct {
		Flds json.RawMessage `json:"flds"`
	}
	if err := json.Unmarshal(modelMeta, &node); err != nil {
		return domain.Model{}, fmt.Errorf("invalid col.models entry for model %d: %w", modelID, err)
	}
	fields, err := parseFieldNamesFromFieldsArray(string(node.Flds))
	if err != nil {
		return domain.Model{}, fmt.Errorf("invalid col.models.flds for model %d: %w", modelID, err)
	}
	return domain.Model{ID: modelID, FieldNames: fields}, nil
}

func parseFieldNamesFromFieldsArray(fieldsJSON string) ([]string, error) {
	if strings.TrimSpace(fieldsJSON) == "" {
		return nil, errors.New("empty fields JSON")
	}
	var withNames []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(fieldsJSON), &withNames); err != nil {
		return nil, err
	}
	fields := make([]string, 0, len(withNames))
	for _, f := range withNames {
		fields = append(fields, f.Name)
	}
	if len(fields) == 0 {
		return nil, errors.New("no fields found")
	}
	return fields, nil
}
