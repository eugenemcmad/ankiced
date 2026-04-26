package application

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	appconfig "ankiced/internal/config"
	"ankiced/internal/domain"
)

type Services struct {
	Cfg appconfig.Settings
	// CfgProvider, when non-nil, returns the *current* effective Settings.
	// It is consulted in preference to the captured Cfg field for any
	// runtime-mutable values (notably DBPath, which can change after a live
	// Reconnect from the HTTP UI). When nil, Cfg is used unchanged.
	CfgProvider func() appconfig.Settings
	Decks       DeckRepository
	Notes       NoteRepository
	Models      ModelRepository
	Backups     BackupStore
	Confirm     ConfirmPrompter
	Diff        DiffRenderer
	Reports     ReportWriter
	Tx          TransactionManager
	Templates   TemplateRegistry
	Now         func() time.Time
}

func (s Services) currentCfg() appconfig.Settings {
	if s.CfgProvider != nil {
		return s.CfgProvider()
	}
	return s.Cfg
}

const confirmApplyChangesPrompt = "Apply changes to database?"

func (s Services) EnsureBackup(ctx context.Context, cfg appconfig.Settings) error {
	if s.Backups == nil {
		return nil
	}
	info, err := s.Backups.CreateBackup(ctx, cfg.DBPath, s.now())
	if err != nil {
		return err
	}
	if info.Path != "" {
		_ = s.CleanupBackups(ctx, cfg) // Best effort cleanup
	}
	return nil
}

func (s Services) CleanupBackups(ctx context.Context, cfg appconfig.Settings) error {
	if s.Backups == nil {
		return nil
	}
	keep := cfg.BackupKeepLastN
	if keep <= 0 {
		keep = 3
	}
	return s.Backups.CleanupBackups(ctx, cfg.DBPath, keep)
}

func (s Services) ListDecks(ctx context.Context) ([]domain.Deck, error) {
	return s.Decks.ListDecks(ctx)
}

func (s Services) SearchDecks(ctx context.Context, search string) ([]domain.Deck, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil, domain.ErrDeckSearchEmpty
	}
	return s.Decks.SearchDecks(ctx, search)
}

func (s Services) RenameDeck(ctx context.Context, deckID int64, newName string) error {
	if err := domain.ValidateDeckRename(newName); err != nil {
		return err
	}
	deckExists, err := s.Decks.DeckExists(ctx, deckID)
	if err != nil {
		return err
	}
	if !deckExists {
		return ErrDeckNotFound
	}
	exists, err := s.Decks.DeckNameExists(ctx, strings.TrimSpace(newName), deckID)
	if err != nil {
		return err
	}
	if exists {
		return domain.ErrDeckNameConflict
	}
	if err := s.EnsureBackup(ctx, s.currentCfg()); err != nil {
		return err
	}
	return s.Decks.RenameDeck(ctx, deckID, strings.TrimSpace(newName))
}

func (s Services) ListNotes(ctx context.Context, filters domain.FilterSet, page domain.Pagination) ([]domain.Note, error) {
	return s.Notes.ListNotes(ctx, filters, page)
}

// CountNotes mirrors ListNotes filtering but returns the total match count
// (without pagination) so HTTP/UI callers can render accurate page counters.
func (s Services) CountNotes(ctx context.Context, filters domain.FilterSet) (int64, error) {
	return s.Notes.CountNotes(ctx, filters)
}

func (s Services) GetNote(ctx context.Context, noteID int64) (domain.Note, error) {
	note, err := s.Notes.GetNote(ctx, noteID)
	if err != nil {
		return domain.Note{}, err
	}
	model, err := s.Models.GetModelByID(ctx, note.ModelID)
	if err != nil {
		return domain.Note{}, err
	}
	values := strings.Split(note.RawFlds, domain.FieldSeparator)
	fields, err := domain.MapFields(values, model)
	if err != nil {
		return domain.Note{}, err
	}
	note.Fields = fields
	return note, nil
}

func (s Services) UpdateNote(ctx context.Context, noteID int64, fields []domain.NoteField) error {
	return s.updateNote(ctx, noteID, fields, true)
}

func (s Services) updateNote(ctx context.Context, noteID int64, fields []domain.NoteField, createBackup bool) error {
	note, err := s.Notes.GetNote(ctx, noteID)
	if err != nil {
		return err
	}
	model, err := s.Models.GetModelByID(ctx, note.ModelID)
	if err != nil {
		return err
	}
	if _, err := domain.MapFields(domain.JoinFieldValues(fields), model); err != nil {
		return err
	}
	if createBackup {
		if err := s.EnsureBackup(ctx, s.currentCfg()); err != nil {
			return err
		}
	}
	note.Fields = fields
	note.RawFlds = strings.Join(domain.JoinFieldValues(fields), domain.FieldSeparator)
	note.Mod = s.now().Unix()
	note.USN = -1
	return s.Notes.UpdateNote(ctx, note)
}

func (s Services) PreviewCleaner(ctx context.Context, noteID int64, templateID string) ([]domain.DiffRecord, error) {
	template, err := s.template(templateID)
	if err != nil {
		return nil, err
	}
	note, err := s.GetNote(ctx, noteID)
	if err != nil {
		return nil, err
	}
	records := make([]domain.DiffRecord, 0)
	for _, f := range note.Fields {
		cleaned, err := template.Apply(f.Value)
		if err != nil {
			return nil, err
		}
		if cleaned != f.Value {
			records = append(records, domain.DiffRecord{
				NoteID:    noteID,
				FieldName: f.Name,
				Before:    f.Value,
				After:     cleaned,
			})
		}
	}
	return records, nil
}

func (s Services) RunCleaner(ctx context.Context, cfg appconfig.Settings, deckID int64, dryRun bool, templateID string, progress func(domain.CleanerProgress)) (string, domain.DryRunSummary, error) {
	template, err := s.template(templateID)
	if err != nil {
		return "", domain.DryRunSummary{}, err
	}
	noteIDs, err := s.Notes.ListNoteIDsByDeck(ctx, deckID)
	if err != nil {
		return "", domain.DryRunSummary{}, err
	}
	reportProgress(progress, domain.CleanerProgress{Total: len(noteIDs), Stage: "started"})
	if !dryRun && !cfg.ForceApply {
		ok, err := s.Confirm.Confirm(ctx, confirmApplyChangesPrompt)
		if err != nil {
			return "", domain.DryRunSummary{}, err
		}
		if !ok {
			return "", domain.DryRunSummary{}, ErrOperationCancelled
		}
	}

	type processedNote struct {
		noteID  int64
		fields  []domain.NoteField
		records []domain.DiffRecord
	}
	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan int64)
	// Bounded buffer keeps memory predictable for huge decks while still letting
	// workers progress when the consumer is briefly slower.
	resultsBuffer := workers * 4
	if resultsBuffer < 1 {
		resultsBuffer = 1
	}
	results := make(chan processedNote, resultsBuffer)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	ctxRun, cancel := context.WithCancel(ctx)
	defer cancel()
	// reportErr is shared by workers/feeder to report fatal errors and cancel
	// the rest of the pipeline. It is safe to call from any goroutine.
	reportErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
		cancel()
	}
	// Worker stage: read + transform notes in parallel.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					reportErr(fmt.Errorf("cleaner worker panicked: %v", r))
				}
			}()
			for id := range jobs {
				note, err := s.GetNote(ctxRun, id)
				if err != nil {
					reportErr(err)
					return
				}
				changed := false
				nextFields := make([]domain.NoteField, 0, len(note.Fields))
				var diffs []domain.DiffRecord
				for _, field := range note.Fields {
					cleaned, err := template.Apply(field.Value)
					if err != nil {
						reportErr(err)
						return
					}
					nextFields = append(nextFields, domain.NoteField{Name: field.Name, Value: cleaned})
					if cleaned != field.Value {
						changed = true
						diffs = append(diffs, domain.DiffRecord{NoteID: note.ID, FieldName: field.Name, Before: field.Value, After: cleaned})
					}
				}
				if !changed {
					results <- processedNote{noteID: id}
					continue
				}
				results <- processedNote{noteID: id, fields: nextFields, records: diffs}
			}
		}()
	}
	// Feeder stage: push note IDs until done/cancelled. Recover from panics
	// so a faulty data source cannot escape this goroutine and crash the
	// surrounding HTTP/CLI process; the panic is converted to errCh.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				reportErr(fmt.Errorf("cleaner feeder panicked: %v", r))
			}
			close(jobs)
			wg.Wait()
			close(results)
		}()
		for _, id := range noteIDs {
			select {
			case <-ctxRun.Done():
				return
			case jobs <- id:
			}
		}
	}()

	records := make([]domain.DiffRecord, 0)
	summary := domain.DryRunSummary{}
	toWrite := make([]processedNote, 0)
	for res := range results {
		summary.Processed++
		if len(res.records) == 0 {
			summary.Skipped++
			reportProgress(progress, progressFromSummary(len(noteIDs), "transforming", summary))
			continue
		}
		summary.Changed++
		records = append(records, res.records...)
		toWrite = append(toWrite, res)
		reportProgress(progress, progressFromSummary(len(noteIDs), "transforming", summary))
	}
	if summary.Processed == len(noteIDs) {
		reportProgress(progress, progressFromSummary(len(noteIDs), "transformed", summary))
	} else {
		reportProgress(progress, progressFromSummary(len(noteIDs), "transforming", summary))
	}
	select {
	case err := <-errCh:
		summary.Errors++
		reportProgress(progress, progressFromSummary(len(noteIDs), "failed", summary))
		return "", summary, err
	default:
	}
	// Writer stage: apply updates in batches of `batchSize`, one transaction
	// per batch, as required by tech-spec 6.7. Each batch commits before the
	// next begins so memory usage stays bounded and a failed batch leaves
	// previously committed batches intact.
	if !dryRun {
		reportProgress(progress, progressFromSummary(len(noteIDs), "writing", summary))
		if err := s.EnsureBackup(ctx, cfg); err != nil {
			reportProgress(progress, progressFromSummary(len(noteIDs), "failed", summary))
			return "", summary, err
		}
		batchSize := 1000
		for i := 0; i < len(toWrite); i += batchSize {
			end := i + batchSize
			if end > len(toWrite) {
				end = len(toWrite)
			}
			batch := toWrite[i:end]
			if err := s.Tx.WithTx(ctx, func(txCtx context.Context) error {
				for _, item := range batch {
					if err := s.updateNote(txCtx, item.noteID, item.fields, false); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				reportProgress(progress, progressFromSummary(len(noteIDs), "failed", summary))
				return "", summary, err
			}
		}
	}

	if cfg.ReportFile != "" && s.Reports != nil {
		if err := s.Reports.WriteReport(ctx, cfg.ReportFile, records, summary); err != nil {
			reportProgress(progress, progressFromSummary(len(noteIDs), "failed", summary))
			return "", summary, fmt.Errorf("%w: %v", ErrReportWriteFailed, err)
		}
	}
	reportProgress(progress, progressFromSummary(len(noteIDs), "finished", summary))
	return s.Diff.Render(records, summary, cfg.FullDiff), summary, nil
}

func progressFromSummary(total int, stage string, summary domain.DryRunSummary) domain.CleanerProgress {
	return domain.CleanerProgress{
		Total:     total,
		Processed: summary.Processed,
		Changed:   summary.Changed,
		Skipped:   summary.Skipped,
		Errors:    summary.Errors,
		Stage:     stage,
		Summary:   summary,
	}
}

func reportProgress(report func(domain.CleanerProgress), progress domain.CleanerProgress) {
	if report != nil {
		report(progress)
	}
}

func (s Services) template(templateID string) (domain.ActionTemplate, error) {
	if s.Templates == nil {
		return nil, ErrTemplateNotFound
	}
	if strings.TrimSpace(templateID) == "" {
		template, err := s.Templates.Default()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrTemplateNotFound, err)
		}
		return template, nil
	}
	template, err := s.Templates.Get(strings.TrimSpace(templateID))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateNotFound, err)
	}
	return template, nil
}

func (s Services) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
