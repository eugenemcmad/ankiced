package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"ankiced/internal/application"
	appconfig "ankiced/internal/config"
	"ankiced/internal/domain"
	"ankiced/internal/infrastructure/sanitize"
	"ankiced/internal/presentation"
)

type App struct {
	Svc application.Services
	Cfg appconfig.Settings
	In  io.Reader
	Out io.Writer
}

func (a App) Run(ctx context.Context) error {
	reader := bufio.NewReader(a.In)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := a.writeln(menuTitle); err != nil {
			return err
		}
		if err := a.writeln(menuDecks); err != nil {
			return err
		}
		if err := a.writeln(menuRename); err != nil {
			return err
		}
		if err := a.writeln(menuSearchDecks); err != nil {
			return err
		}
		if err := a.writeln(menuNotes); err != nil {
			return err
		}
		if err := a.writeln(menuEditNote); err != nil {
			return err
		}
		if err := a.writeln(menuPreviewCleaner); err != nil {
			return err
		}
		if err := a.writeln(menuDryRunCleaner); err != nil {
			return err
		}
		if err := a.writeln(menuApplyCleaner); err != nil {
			return err
		}
		if err := a.writeln(menuFindNoteByID); err != nil {
			return err
		}
		if err := a.writeln(menuSearchNotesAll); err != nil {
			return err
		}
		if err := a.writeln(menuExit); err != nil {
			return err
		}
		if err := a.write(promptCursor); err != nil {
			return err
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		var actionErr error
		switch strings.TrimSpace(line) {
		case "1":
			actionErr = a.listDecks(ctx)
		case "2":
			actionErr = a.renameDeck(ctx, reader)
		case "10":
			actionErr = a.searchDecks(ctx, reader)
		case "3":
			actionErr = a.listNotes(ctx, reader)
		case "4":
			actionErr = a.editNote(ctx, reader)
		case "5":
			actionErr = a.previewCleaner(ctx, reader)
		case "6":
			actionErr = a.runCleaner(ctx, reader, true)
		case "7":
			actionErr = a.runCleaner(ctx, reader, false)
		case "8":
			actionErr = a.findNoteByID(ctx, reader)
		case "9":
			actionErr = a.searchNotesAllDecks(ctx, reader)
		case "0":
			return nil
		default:
			if err := a.writeln(menuUnknownOption); err != nil {
				return err
			}
		}
		if actionErr != nil {
			if shouldExitOnError(actionErr) {
				return actionErr
			}
			if a.Cfg.Verbose {
				if err := a.writef("error: %s\n", presentation.FormatDebugError(actionErr)); err != nil {
					return err
				}
				continue
			}
			if err := a.writef("error: %s\n", presentation.FormatError(actionErr)); err != nil {
				return err
			}
		}
	}
}

func (a App) listDecks(ctx context.Context) error {
	decks, err := a.Svc.ListDecks(ctx)
	if err != nil {
		return err
	}
	for _, d := range decks {
		if err := a.writef("id=%d name=%s cards=%d\n", d.ID, d.Name, d.CardCount); err != nil {
			return err
		}
	}
	return nil
}

func (a App) renameDeck(ctx context.Context, r *bufio.Reader) error {
	if err := a.write("deck id: "); err != nil {
		return err
	}
	deckID, err := readInt64(r)
	if err != nil {
		return err
	}
	if err := a.write("new name: "); err != nil {
		return err
	}
	name, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	return a.Svc.RenameDeck(ctx, deckID, strings.TrimSpace(name))
}

func (a App) searchDecks(ctx context.Context, r *bufio.Reader) error {
	if err := a.write("search text: "); err != nil {
		return err
	}
	search, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	decks, err := a.Svc.SearchDecks(ctx, search)
	if err != nil {
		return err
	}
	for _, d := range decks {
		if err := a.writef("id=%d name=%s cards=%d\n", d.ID, d.Name, d.CardCount); err != nil {
			return err
		}
	}
	return nil
}

func (a App) listNotes(ctx context.Context, r *bufio.Reader) error {
	if err := a.write("deck id: "); err != nil {
		return err
	}
	deckID, err := readInt64(r)
	if err != nil {
		return err
	}
	limit, offset, modFrom, modTo, search, err := a.readPaginationAndSearch(r, "search text (optional): ")
	if err != nil {
		return err
	}
	notes, err := a.Svc.ListNotes(ctx, domain.FilterSet{
		DeckID:      deckID,
		SearchText:  search,
		ModFromUnix: modFrom,
		ModToUnix:   modTo,
	}, domain.Pagination{
		Limit:  int(limit),
		Offset: int(offset),
	})
	if err != nil {
		return err
	}
	return a.writeNoteListLines(notes)
}

func (a App) findNoteByID(ctx context.Context, r *bufio.Reader) error {
	if err := a.write("note id: "); err != nil {
		return err
	}
	noteID, err := readInt64(r)
	if err != nil {
		return err
	}
	if noteID <= 0 {
		return domain.ErrInvalidNoteID
	}
	notes, err := a.Svc.ListNotes(ctx, domain.FilterSet{NoteID: noteID}, domain.Pagination{Limit: 1, Offset: 0})
	if err != nil {
		return err
	}
	return a.writeNoteListLines(notes)
}

func (a App) searchNotesAllDecks(ctx context.Context, r *bufio.Reader) error {
	limit, offset, modFrom, modTo, search, err := a.readPaginationAndSearch(r, "search text (required): ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(search) == "" {
		return domain.ErrInvalidNoteListFilters
	}
	notes, err := a.Svc.ListNotes(ctx, domain.FilterSet{
		DeckID:      0,
		SearchText:  strings.TrimSpace(search),
		ModFromUnix: modFrom,
		ModToUnix:   modTo,
	}, domain.Pagination{
		Limit:  int(limit),
		Offset: int(offset),
	})
	if err != nil {
		return err
	}
	return a.writeNoteListLines(notes)
}

func (a App) readPaginationAndSearch(r *bufio.Reader, searchPrompt string) (limit, offset, modFrom, modTo int64, search string, err error) {
	defaultLimit := a.Cfg.DefaultPageSize
	if defaultLimit <= 0 {
		defaultLimit = 10
	}
	if err = a.writef("limit (default %d): ", defaultLimit); err != nil {
		return
	}
	limit, err = readInt64(r)
	if err != nil {
		return
	}
	if limit <= 0 {
		limit = int64(defaultLimit)
	}
	if err = a.write("offset (default 0): "); err != nil {
		return
	}
	offset, err = readInt64(r)
	if err != nil {
		return
	}
	if err = a.write("mod from unix (optional): "); err != nil {
		return
	}
	modFrom, err = readInt64(r)
	if err != nil {
		return
	}
	if err = a.write("mod to unix (optional): "); err != nil {
		return
	}
	modTo, err = readInt64(r)
	if err != nil {
		return
	}
	if err = a.write(searchPrompt); err != nil {
		return
	}
	var line string
	line, err = r.ReadString('\n')
	if err != nil {
		return
	}
	search = strings.TrimSpace(line)
	return
}

func (a App) writeNoteListLines(notes []domain.Note) error {
	for _, n := range notes {
		if err := a.writef("note=%d mod=%d preview=%s\n", n.ID, n.Mod, previewText(n.RawFlds)); err != nil {
			return err
		}
	}
	return nil
}

func (a App) editNote(ctx context.Context, r *bufio.Reader) error {
	if err := a.write("note id: "); err != nil {
		return err
	}
	noteID, err := readInt64(r)
	if err != nil {
		return err
	}
	note, err := a.Svc.GetNote(ctx, noteID)
	if err != nil {
		return err
	}
	updated := make([]domain.NoteField, 0, len(note.Fields))
	for _, f := range note.Fields {
		if err := a.writef("%s current:\n%s\n", f.Name, f.Value); err != nil {
			return err
		}
		if err := a.writef("new value for %s (finish with single line %s):\n", f.Name, multilineTerminator); err != nil {
			return err
		}
		value, err := readMultiline(r)
		if err != nil {
			return err
		}
		updated = append(updated, domain.NoteField{Name: f.Name, Value: value})
	}
	return a.Svc.UpdateNote(ctx, noteID, updated)
}

func (a App) previewCleaner(ctx context.Context, r *bufio.Reader) error {
	if err := a.write("note id: "); err != nil {
		return err
	}
	noteID, err := readInt64(r)
	if err != nil {
		return err
	}
	records, err := a.Svc.PreviewCleaner(ctx, noteID, domain.DefaultActionTemplateID)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if err := a.writef("field=%s\n- %s\n+ %s\n", rec.FieldName, rec.Before, rec.After); err != nil {
			return err
		}
	}
	return nil
}

func (a App) runCleaner(ctx context.Context, r *bufio.Reader, dryRun bool) error {
	if err := a.write("deck id: "); err != nil {
		return err
	}
	deckID, err := readInt64(r)
	if err != nil {
		return err
	}
	out, _, err := a.Svc.RunCleaner(ctx, a.Cfg, deckID, dryRun, domain.DefaultActionTemplateID, nil)
	if err != nil {
		return err
	}
	if err := a.writeln(out); err != nil {
		return err
	}
	return nil
}

func readInt64(r *bufio.Reader) (int64, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, nil
	}
	return strconv.ParseInt(line, 10, 64)
}

func readMultiline(r *bufio.Reader) (string, error) {
	var lines []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		trim := strings.TrimRight(line, "\r\n")
		if trim == multilineTerminator {
			return strings.Join(lines, "\n"), nil
		}
		decoded, err := decodeEscapes(trim)
		if err != nil {
			return "", err
		}
		lines = append(lines, decoded)
	}
}

func decodeEscapes(value string) (string, error) {
	quoted := `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	decoded, err := strconv.Unquote(quoted)
	if err == nil {
		return decoded, nil
	}
	return "", fmt.Errorf("%w: %v", ErrInvalidEscapeSequence, err)
}

func previewText(raw string) string {
	parts := strings.Split(raw, domain.FieldSeparator)
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned = append(cleaned, sanitize.StripAllTags(part))
	}
	value := strings.Join(cleaned, " | ")
	if len(value) > 80 {
		return value[:80] + "..."
	}
	return value
}

func shouldExitOnError(err error) bool {
	return errors.Is(err, io.EOF)
}

func (a App) write(text string) error {
	_, err := fmt.Fprint(a.Out, text)
	return err
}

func (a App) writeln(text string) error {
	_, err := fmt.Fprintln(a.Out, text)
	return err
}

func (a App) writef(format string, args ...any) error {
	_, err := fmt.Fprintf(a.Out, format, args...)
	return err
}
