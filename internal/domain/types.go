package domain

import "time"

const FieldSeparator = "\x1f"
const DefaultActionTemplateID = "html_cleaner"

type ActionTemplate interface {
	ID() string
	Name() string
	Apply(input string) (string, error)
}

type Deck struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CardCount int64  `json:"card_count"`
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type FilterSet struct {
	DeckID      int64  `json:"deck_id"`
	NoteID      int64  `json:"note_id"` // if > 0, list only this note (ignores deck filter)
	SearchText  string `json:"search_text"`
	ModFromUnix int64  `json:"mod_from_unix"`
	ModToUnix   int64  `json:"mod_to_unix"`
}

type Note struct {
	ID      int64       `json:"id"`
	GUID    string      `json:"guid"`
	ModelID int64       `json:"model_id"`
	RawFlds string      `json:"raw_flds"`
	Fields  []NoteField `json:"fields"`
	Mod     int64       `json:"mod"`
	USN     int         `json:"usn"`
}

type NoteField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Model struct {
	ID         int64    `json:"id"`
	FieldNames []string `json:"field_names"`
}

type DiffRecord struct {
	NoteID    int64  `json:"note_id"`
	FieldName string `json:"field_name"`
	Before    string `json:"before"`
	After     string `json:"after"`
}

type DryRunSummary struct {
	Processed int `json:"processed"`
	Changed   int `json:"changed"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
}

type CleanerProgress struct {
	Total     int           `json:"total"`
	Processed int           `json:"processed"`
	Changed   int           `json:"changed"`
	Skipped   int           `json:"skipped"`
	Errors    int           `json:"errors"`
	Stage     string        `json:"stage"`
	Summary   DryRunSummary `json:"summary"`
}

type BackupInfo struct {
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}
