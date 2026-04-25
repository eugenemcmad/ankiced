package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"ankiced/internal/application"
	appconfig "ankiced/internal/config"
	"ankiced/internal/domain"
	"ankiced/internal/presentation"
)

var executablePath = os.Executable

type Server struct {
	Svc    application.Services
	Cfg    appconfig.Settings
	Logger *slog.Logger
	OnExit func()
}

func (s Server) Run(ctx context.Context) error {
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	addr := s.Cfg.HTTPAddr
	if strings.TrimSpace(addr) == "" {
		addr = "127.0.0.1:8080"
	}
	api := newAPIHandler(s.Svc, s.Cfg, logger, s.OnExit)
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("http api listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// Long-lived connections (SSE/log stream) may keep shutdown pending.
			// On app-exit request we prefer deterministic close without surfacing
			// a fatal error to the caller.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				_ = srv.Close()
				return nil
			}
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}

type apiHandler struct {
	svc    application.Services
	cfg    appconfig.Settings
	logger *slog.Logger
	logs   application.LogBroadcaster
	ops    *operationStore
	seq    uint64
	logSeq uint64
	onExit func()
}

func newAPIHandler(svc application.Services, cfg appconfig.Settings, logger *slog.Logger, onExit func()) *apiHandler {
	return &apiHandler{
		svc:    svc,
		cfg:    cfg,
		logger: logger,
		logs:   newLogHub(),
		ops:    newOperationStore(),
		onExit: onExit,
	}
}

func NewHandler(svc application.Services, cfg appconfig.Settings, logger *slog.Logger) http.Handler {
	return newAPIHandler(svc, cfg, logger, nil).routes()
}

func NewHandlerWithExit(svc application.Services, cfg appconfig.Settings, logger *slog.Logger, onExit func()) http.Handler {
	return newAPIHandler(svc, cfg, logger, onExit).routes()
}

func (h *apiHandler) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/api/v1/logs/stream", h.handleLogStream)
	mux.HandleFunc("/api/v1/logs", h.handleLogs)
	mux.HandleFunc("/api/v1/config", h.handleConfig)
	mux.HandleFunc("/api/v1/config/save", h.handleConfigSave)
	mux.HandleFunc("/api/v1/decks", h.handleDecks)
	mux.HandleFunc("/api/v1/decks/search", h.handleDeckSearch)
	mux.HandleFunc("/api/v1/decks/", h.handleDeckByID)
	mux.HandleFunc("/api/v1/notes", h.handleNotes)
	mux.HandleFunc("/api/v1/notes/", h.handleNoteByID)
	mux.HandleFunc("/api/v1/cleaner/preview", h.handleCleanerPreview)
	mux.HandleFunc("/api/v1/cleaner/dry-run", h.handleCleanerDryRun)
	mux.HandleFunc("/api/v1/cleaner/apply", h.handleCleanerApply)
	mux.HandleFunc("/api/v1/operations/", h.handleOperationByID)
	mux.HandleFunc("/api/v1/app/exit", h.handleAppExit)
	return withJSONContentType(mux)
}

func (h *apiHandler) handleAppExit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeMethodNotAllowed(w, "app_exit")
		return
	}
	if h.onExit == nil {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusNotImplemented, "exit action is not enabled", "APP_EXIT_DISABLED", "app exit action is not wired", "app_exit", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	go h.onExit()
}

func (h *apiHandler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			cid := h.correlationID()
			h.writeMessageError(w, http.StatusNotFound, "endpoint not found", "NOT_FOUND", "api endpoint was not found", "not_found", cid)
			return
		}
		http.NotFound(w, r)
		return
	}
	pageSize := h.cfg.DefaultPageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, indexViewModel{DBPath: h.cfg.DBPath, PageSize: pageSize}); err != nil {
		h.logger.Warn("render index template", "error", err)
	}
}

func (h *apiHandler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *apiHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, "get_config")
		return
	}
	cid := h.correlationID()
	h.log("info", "get_config", cid, "request started", nil)
	h.writeJSON(w, http.StatusOK, map[string]any{"config": h.configSnapshot()})
	h.log("info", "get_config", cid, "request finished", nil)
}

func (h *apiHandler) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeMethodNotAllowed(w, "save_config")
		return
	}
	cid := h.correlationID()
	h.log("info", "save_config", cid, "request started", nil)
	path, err := appConfigPath()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err, "save_config", cid)
		return
	}
	var req struct {
		DBPath string `json:"db_path"`
	}
	if err := decodeJSON(r, &req); err == nil && req.DBPath != "" {
		if req.DBPath != h.cfg.DBPath {
			if reconnector, ok := h.svc.Tx.(interface{ Reconnect(string) error }); ok {
				if err := reconnector.Reconnect(req.DBPath); err != nil {
					h.logger.Error("failed to reconnect to database", "error", err)
					h.writeError(w, http.StatusInternalServerError, err, "save_config", cid)
					return
				}
			}
			h.cfg.DBPath = req.DBPath
		}
	}

	cfg := h.configSnapshot()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err, "save_config", cid)
		return
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		h.writeError(w, http.StatusInternalServerError, err, "save_config", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path, "config": cfg})
	h.log("info", "save_config", cid, "request finished", map[string]any{"path": path})
}

func (h *apiHandler) configSnapshot() configSnapshot {
	return configSnapshot{
		DBPath:            h.cfg.DBPath,
		AnkiAccount:       h.cfg.AnkiAccount,
		HTTPAddr:          h.cfg.HTTPAddr,
		BackupKeepLastN:   h.cfg.BackupKeepLastN,
		Workers:           h.cfg.Workers,
		ForceApply:        h.cfg.ForceApply,
		Verbose:           h.cfg.Verbose,
		FullDiff:          h.cfg.FullDiff,
		ReportFile:        h.cfg.ReportFile,
		DefaultPageSize:   h.cfg.DefaultPageSize,
		PragmaBusyTimeout: h.cfg.PragmaBusyTimeout,
		PragmaJournalMode: h.cfg.PragmaJournalMode,
		PragmaSynchronous: h.cfg.PragmaSynchronous,
	}
}

func (h *apiHandler) handleDecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, "list_decks")
		return
	}
	cid := h.correlationID()
	h.log("info", "list_decks", cid, "request started", nil)
	decks, err := h.svc.ListDecks(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err, "list_decks", cid)
		return
	}
	total := len(decks)
	limit := int(parseInt64Default(r.URL.Query().Get("limit"), 0))
	offset := int(parseInt64Default(r.URL.Query().Get("offset"), 0))
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	page := decks[offset:]
	if limit > 0 && limit < len(page) {
		page = page[:limit]
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"items":  page,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
	h.log("info", "list_decks", cid, "request finished", map[string]any{"count": len(page), "total": total})
}

func (h *apiHandler) handleDeckSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, "search_decks")
		return
	}
	cid := h.correlationID()
	query := r.URL.Query().Get("q")
	h.log("info", "search_decks", cid, "request started", map[string]any{"q": query})
	decks, err := h.svc.SearchDecks(r.Context(), query)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrDeckSearchEmpty) {
			status = http.StatusBadRequest
		}
		h.writeError(w, status, err, "search_decks", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": decks})
	h.log("info", "search_decks", cid, "request finished", map[string]any{"count": len(decks)})
}

func (h *apiHandler) handleDeckByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		h.writeMethodNotAllowed(w, "rename_deck")
		return
	}
	id, err := parsePathID(r.URL.Path, "/api/v1/decks/")
	if err != nil {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "invalid deck id", "DECK_ID_INVALID", err.Error(), "rename_deck", cid)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON", err.Error(), "rename_deck", cid)
		return
	}
	cid := h.correlationID()
	h.log("info", "rename_deck", cid, "request started", map[string]any{"deck_id": id})
	if err := h.svc.RenameDeck(r.Context(), id, req.Name); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, domain.ErrEmptyDeckName), errors.Is(err, domain.ErrDeckNameInvalid), errors.Is(err, domain.ErrDeckNameTooLong), errors.Is(err, domain.ErrDeckNameConflict):
			status = http.StatusBadRequest
		case errors.Is(err, application.ErrDeckNotFound):
			status = http.StatusNotFound
		}
		h.writeError(w, status, err, "rename_deck", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	h.log("info", "rename_deck", cid, "request finished", nil)
}

func (h *apiHandler) handleNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, "list_notes")
		return
	}
	filters := domain.FilterSet{
		DeckID:      parseInt64Default(r.URL.Query().Get("deck_id"), 0),
		NoteID:      parseInt64Default(r.URL.Query().Get("note_id"), 0),
		SearchText:  strings.TrimSpace(r.URL.Query().Get("search_text")),
		ModFromUnix: parseInt64Default(r.URL.Query().Get("mod_from"), 0),
		ModToUnix:   parseInt64Default(r.URL.Query().Get("mod_to"), 0),
	}
	page := domain.Pagination{
		Limit:  int(parseInt64Default(r.URL.Query().Get("limit"), int64(h.cfg.DefaultPageSize))),
		Offset: int(parseInt64Default(r.URL.Query().Get("offset"), 0)),
	}
	if page.Limit <= 0 {
		page.Limit = 10
	}
	cid := h.correlationID()
	h.log("info", "list_notes", cid, "request started", map[string]any{
		"deck_id": filters.DeckID, "note_id": filters.NoteID, "search_text": filters.SearchText,
	})
	notes, err := h.svc.ListNotes(r.Context(), filters, page)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrInvalidNoteListFilters) || errors.Is(err, domain.ErrInvalidNoteID) {
			status = http.StatusBadRequest
		}
		h.writeError(w, status, err, "list_notes", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": notes, "limit": page.Limit, "offset": page.Offset})
	h.log("info", "list_notes", cid, "request finished", map[string]any{"count": len(notes)})
}

func (h *apiHandler) handleNoteByID(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r.URL.Path, "/api/v1/notes/")
	if err != nil {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "invalid note id", "NOTE_ID_INVALID", err.Error(), "note_by_id", cid)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.handleGetNote(w, r, id)
	case http.MethodPatch:
		h.handlePatchNote(w, r, id)
	default:
		h.writeMethodNotAllowed(w, "note_by_id")
	}
}

func (h *apiHandler) handleGetNote(w http.ResponseWriter, r *http.Request, id int64) {
	cid := h.correlationID()
	h.log("info", "get_note", cid, "request started", map[string]any{"note_id": id})
	note, err := h.svc.GetNote(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, application.ErrNoteNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, application.ErrModelNotFound) {
			status = http.StatusNotFound
		}
		h.writeError(w, status, err, "get_note", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, note)
	h.log("info", "get_note", cid, "request finished", nil)
}

func (h *apiHandler) handlePatchNote(w http.ResponseWriter, r *http.Request, id int64) {
	var req struct {
		Fields []domain.NoteField `json:"fields"`
	}
	if err := decodeJSON(r, &req); err != nil {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON", err.Error(), "update_note", cid)
		return
	}
	cid := h.correlationID()
	h.log("info", "update_note", cid, "request started", map[string]any{"note_id": id})
	if err := h.svc.UpdateNote(r.Context(), id, req.Fields); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, domain.ErrFieldCountInvalid):
			status = http.StatusBadRequest
		case errors.Is(err, application.ErrNoteNotFound), errors.Is(err, application.ErrModelNotFound):
			status = http.StatusNotFound
		}
		h.writeError(w, status, err, "update_note", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	h.log("info", "update_note", cid, "request finished", nil)
}

func (h *apiHandler) handleCleanerPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeMethodNotAllowed(w, "cleaner_preview")
		return
	}
	var req struct {
		NoteID     int64  `json:"note_id"`
		TemplateID string `json:"template_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON", err.Error(), "cleaner_preview", cid)
		return
	}
	cid := h.correlationID()
	h.log("info", "cleaner_preview", cid, "request started", map[string]any{"note_id": req.NoteID})
	records, err := h.svc.PreviewCleaner(r.Context(), req.NoteID, req.TemplateID)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, application.ErrNoteNotFound), errors.Is(err, application.ErrModelNotFound):
			status = http.StatusNotFound
		case errors.Is(err, application.ErrTemplateNotFound):
			status = http.StatusBadRequest
		}
		h.writeError(w, status, err, "cleaner_preview", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"records": records})
	h.log("info", "cleaner_preview", cid, "request finished", map[string]any{"changed_fields": len(records)})
}

func (h *apiHandler) handleCleanerDryRun(w http.ResponseWriter, r *http.Request) {
	h.handleCleanerRun(w, r, true)
}

func (h *apiHandler) handleCleanerApply(w http.ResponseWriter, r *http.Request) {
	h.handleCleanerRun(w, r, false)
}

func (h *apiHandler) handleCleanerRun(w http.ResponseWriter, r *http.Request, dryRun bool) {
	op := "cleaner_apply"
	if dryRun {
		op = "cleaner_dry_run"
	}
	if r.Method != http.MethodPost {
		h.writeMethodNotAllowed(w, op)
		return
	}
	var req struct {
		DeckID   int64  `json:"deck_id"`
		FullDiff bool   `json:"full_diff"`
		Confirm  bool   `json:"confirm"`
		Workers  int    `json:"workers"`
		Report   string `json:"report_file"`
		Template string `json:"template_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON", err.Error(), op, cid)
		return
	}
	if req.DeckID <= 0 {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "deck_id must be positive", "DECK_ID_INVALID", "deck_id must be greater than zero", op, cid)
		return
	}
	if !dryRun && !req.Confirm {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "confirm=true is required for apply", "CONFIRM_REQUIRED", "apply requires explicit confirmation", op, cid)
		return
	}

	cid := h.correlationID()
	h.log("info", op, cid, "request started", map[string]any{"deck_id": req.DeckID, "dry_run": dryRun})
	cfg := h.cfg
	cfg.FullDiff = req.FullDiff
	cfg.ForceApply = req.Confirm
	if req.Workers > 0 {
		cfg.Workers = req.Workers
	}
	if req.Report != "" {
		cfg.ReportFile = req.Report
	}
	operation := h.ops.create(cid, op)
	go func() {
		h.log("info", op, cid, "operation started", map[string]any{"deck_id": req.DeckID, "dry_run": dryRun})
		out, summary, err := h.svc.RunCleaner(context.Background(), cfg, req.DeckID, dryRun, req.Template, func(progress domain.CleanerProgress) {
			h.ops.progress(operation.ID, progress)
			if progress.Stage == "started" || progress.Stage == "writing" || progress.Stage == "finished" || progress.Stage == "failed" {
				h.log("info", op, cid, "operation progress", map[string]any{
					"stage": progress.Stage, "processed": progress.Processed, "total": progress.Total,
					"changed": progress.Changed, "skipped": progress.Skipped, "errors": progress.Errors,
				})
			}
		})
		if err != nil {
			h.ops.fail(operation.ID, err)
			h.log("error", op, cid, "operation failed", map[string]any{"error": presentation.FormatDebugError(err)})
			return
		}
		h.ops.succeed(operation.ID, out, summary)
		h.log("info", op, cid, "operation finished", map[string]any{"changed": summary.Changed, "errors": summary.Errors})
	}()
	h.writeJSON(w, http.StatusAccepted, map[string]any{
		"operation_id": cid,
		"status":       operation.Status,
		"dry_run":      dryRun,
		"status_url":   "/api/v1/operations/" + cid,
	})
	h.log("info", op, cid, "request accepted", map[string]any{"operation_id": cid})
}

func (h *apiHandler) handleOperationByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, "get_operation")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
	if id == "" || strings.Contains(id, "/") {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusBadRequest, "invalid operation id", "OPERATION_ID_INVALID", "operation id path segment is invalid", "get_operation", cid)
		return
	}
	op, ok := h.ops.get(id)
	if !ok {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusNotFound, "operation not found", "OPERATION_NOT_FOUND", "operation id was not found", "get_operation", cid)
		return
	}
	h.writeJSON(w, http.StatusOK, op)
}

func (h *apiHandler) handleLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, "logs_stream")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		cid := h.correlationID()
		h.writeMessageError(w, http.StatusInternalServerError, "streaming unsupported", "STREAMING_UNSUPPORTED", "response writer does not implement http.Flusher", "logs_stream", cid)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	// Reconnect support: replay any events the client missed.
	since := parseInt64Default(r.Header.Get("Last-Event-ID"), 0)
	if since == 0 {
		since = parseInt64Default(r.URL.Query().Get("since_id"), 0)
	}
	if since < 0 {
		since = 0
	}

	ch, cancelSub := h.logs.Subscribe()
	defer cancelSub()

	if since > 0 {
		for _, event := range h.logs.Since(since) {
			if !writeSSEEvent(w, event) {
				return
			}
			flusher.Flush()
		}
	}

	pingInterval := 15 * time.Second
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pingTicker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-ch:
			if !ok {
				return
			}
			if !writeSSEEvent(w, event) {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, event application.LogEvent) bool {
	payload, err := json.Marshal(event)
	if err != nil {
		return true // skip this event, keep the stream alive
	}
	if _, err := fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", event.ID, payload); err != nil {
		return false
	}
	return true
}

func (h *apiHandler) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, "logs_list")
		return
	}
	sinceID := parseInt64Default(r.URL.Query().Get("since_id"), 0)
	if sinceID < 0 {
		sinceID = 0
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"items": h.logs.Since(sinceID),
	})
}
