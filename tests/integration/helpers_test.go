package integration

import (
	"bytes"
	"strings"

	"ankiced/internal/application"
	"ankiced/internal/infrastructure/render"
	"ankiced/internal/infrastructure/sanitize"
	sqliteinfra "ankiced/internal/infrastructure/sqlite"
	"ankiced/internal/interfaces/cli"
)

func setupTestServices(db *sqliteinfra.DB, confirm application.ConfirmPrompter) application.Services {
	if confirm == nil {
		confirm = cli.Prompter{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}}
	}
	return application.Services{
		Decks:     sqliteinfra.NewDeckRepo(db),
		Notes:     sqliteinfra.NewNoteRepo(db),
		Models:    sqliteinfra.NewModelRepo(db),
		Diff:      render.DiffRenderer{},
		Confirm:   confirm,
		Reports:   render.JSONReportWriter{},
		Tx:        db,
		Templates: sanitize.NewTemplateRegistry(),
	}
}
