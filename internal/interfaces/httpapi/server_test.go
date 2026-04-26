package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ankiced/internal/application"
	appconfig "ankiced/internal/config"
	"ankiced/internal/domain"
	"ankiced/internal/infrastructure/sanitize"
)

type stubDeckRepo struct {
	renamedID   int64
	renamedName string
	missing     bool
}

func (s *stubDeckRepo) ListDecks(context.Context) ([]domain.Deck, error) {
	return []domain.Deck{{ID: 1, Name: "Default", CardCount: 2}}, nil
}
func (s *stubDeckRepo) SearchDecks(_ context.Context, search string) ([]domain.Deck, error) {
	if strings.TrimSpace(search) == "" {
		return nil, domain.ErrDeckSearchEmpty
	}
	return []domain.Deck{{ID: 2, Name: "Filtered", CardCount: 3}}, nil
}
func (s *stubDeckRepo) DeckNameExists(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (s *stubDeckRepo) DeckExists(context.Context, int64) (bool, error) {
	return !s.missing, nil
}
func (s *stubDeckRepo) RenameDeck(_ context.Context, id int64, name string) error {
	s.renamedID = id
	s.renamedName = name
	return nil
}

type paginatedDeckRepo struct {
	decks []domain.Deck
}

func (r *paginatedDeckRepo) ListDecks(context.Context) ([]domain.Deck, error) {
	return r.decks, nil
}
func (r *paginatedDeckRepo) SearchDecks(context.Context, string) ([]domain.Deck, error) {
	return r.decks, nil
}
func (r *paginatedDeckRepo) DeckNameExists(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (r *paginatedDeckRepo) DeckExists(context.Context, int64) (bool, error) {
	return true, nil
}
func (r *paginatedDeckRepo) RenameDeck(context.Context, int64, string) error { return nil }

func makeDecks(n int) []domain.Deck {
	out := make([]domain.Deck, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, domain.Deck{ID: int64(i), Name: "Deck " + strconv.Itoa(i), CardCount: int64(i)})
	}
	return out
}

type stubNoteRepo struct {
	missing bool
}

func (stubNoteRepo) ListNotes(context.Context, domain.FilterSet, domain.Pagination) ([]domain.Note, error) {
	return []domain.Note{{ID: 100, RawFlds: "front\x1fback", Mod: 1}}, nil
}
func (stubNoteRepo) CountNotes(context.Context, domain.FilterSet) (int64, error) {
	return 1, nil
}
func (s stubNoteRepo) GetNote(_ context.Context, id int64) (domain.Note, error) {
	if s.missing {
		return domain.Note{}, application.ErrNoteNotFound
	}
	return domain.Note{ID: id, ModelID: 10, RawFlds: "<u>front</u>\x1fback"}, nil
}
func (stubNoteRepo) UpdateNote(context.Context, domain.Note) error { return nil }
func (stubNoteRepo) ListNoteIDsByDeck(context.Context, int64) ([]int64, error) {
	return []int64{100}, nil
}

type stubModelRepo struct{}

func (stubModelRepo) GetModelByID(context.Context, int64) (domain.Model, error) {
	return domain.Model{ID: 10, FieldNames: []string{"Front", "Back"}}, nil
}

type stubTx struct{}

func (stubTx) WithTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type stubDiff struct{}

func (stubDiff) Render(records []domain.DiffRecord, summary domain.DryRunSummary, _ bool) string {
	return "summary"
}

func TestDecksEndpoint(t *testing.T) {
	svc := application.Services{
		Decks:  &stubDeckRepo{},
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/decks", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "\"Default\"") {
		t.Fatalf("expected deck in response, got %s", string(body))
	}
}

func TestDecksEndpointHonorsLimitAndOffset(t *testing.T) {
	svc := application.Services{
		Decks:  &paginatedDeckRepo{decks: makeDecks(5)},
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/decks?limit=2&offset=1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Items  []map[string]any `json:"items"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Total != 5 || payload.Limit != 2 || payload.Offset != 1 {
		t.Fatalf("unexpected pagination metadata: %+v", payload)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(payload.Items))
	}
	if payload.Items[0]["id"].(float64) != 2 || payload.Items[1]["id"].(float64) != 3 {
		t.Fatalf("unexpected page slice: %+v", payload.Items)
	}
}

func TestDeckSearchEmptyReturnsBadRequest(t *testing.T) {
	svc := application.Services{
		Decks:  &stubDeckRepo{},
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/decks/search?q=", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["code"] != "DECK_SEARCH_EMPTY" {
		t.Fatalf("expected DECK_SEARCH_EMPTY code, got %#v", payload["code"])
	}
	if payload["recommended_action"] == "" {
		t.Fatalf("expected recommended_action, got %#v", payload)
	}
}

func TestAPIUnknownRouteUsesErrorContract(t *testing.T) {
	handler := NewHandlerWithExit(context.Background(), application.Services{}, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for _, key := range []string{"message", "recommended_action", "code", "details", "correlation_id"} {
		if payload[key] == "" {
			t.Fatalf("expected %s in error payload, got %#v", key, payload)
		}
	}
	if payload["code"] != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND code, got %#v", payload["code"])
	}
}

func TestMethodNotAllowedUsesErrorContract(t *testing.T) {
	handler := NewHandlerWithExit(context.Background(), application.Services{}, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/decks", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["code"] != "METHOD_NOT_ALLOWED" || payload["recommended_action"] == "" || payload["correlation_id"] == "" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestAppExitInvokesConfiguredCallback(t *testing.T) {
	exited := make(chan struct{}, 1)
	handler := NewHandlerWithExit(context.Background(), application.Services{}, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), func() {
		exited <- struct{}{}
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/app/exit", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("exit callback was not invoked")
	}
}

func TestLogsListReturnsIncrementalEntries(t *testing.T) {
	h := newAPIHandler(application.Services{}, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	h.log("info", "unit_test", "cid-1", "first", map[string]any{"k": "v1"})
	h.log("warn", "unit_test", "cid-2", "second", map[string]any{"k": "v2"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?since_id=0", nil)
	rr := httptest.NewRecorder()
	h.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(payload.Items) < 2 {
		t.Fatalf("expected at least 2 log items, got %d", len(payload.Items))
	}
	firstID, ok1 := payload.Items[0]["id"].(float64)
	secondID, ok2 := payload.Items[1]["id"].(float64)
	if !ok1 || !ok2 || secondID <= firstID {
		t.Fatalf("expected increasing log ids, got first=%v second=%v", payload.Items[0]["id"], payload.Items[1]["id"])
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/logs?since_id="+strconv.Itoa(int(firstID)), nil)
	rr2 := httptest.NewRecorder()
	h.routes().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr2.Code, rr2.Body.String())
	}
	var payload2 struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(rr2.Body).Decode(&payload2); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(payload2.Items) == 0 {
		t.Fatalf("expected at least one incremental log item")
	}
	if id, _ := payload2.Items[0]["id"].(float64); id <= firstID {
		t.Fatalf("expected incremental id > %v, got %v", firstID, id)
	}
}

func TestLogStreamReplaysFromLastEventID(t *testing.T) {
	h := newAPIHandler(application.Services{}, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	h.log("info", "unit_test", "cid-1", "first", nil)
	h.log("info", "unit_test", "cid-2", "second", nil)
	h.log("info", "unit_test", "cid-3", "third", nil)

	srv := httptest.NewServer(h.routes())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/logs/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Last-Event-ID", "1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	deadline := time.After(2 * time.Second)
	collected := strings.Builder{}
	done := make(chan struct{})
	go func() {
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				collected.Write(buf[:n])
				if strings.Contains(collected.String(), "\"third\"") {
					close(done)
					return
				}
			}
			if readErr != nil {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-deadline:
		t.Fatalf("did not receive replayed events; got: %s", collected.String())
	}

	body := collected.String()
	if !strings.Contains(body, "id: 2") || !strings.Contains(body, "\"second\"") {
		t.Fatalf("expected event id 2 and second message in stream: %s", body)
	}
	if !strings.Contains(body, "\"third\"") {
		t.Fatalf("expected third message in stream: %s", body)
	}
	if strings.Contains(body, "\"first\"") {
		t.Fatalf("did not expect first message after Last-Event-ID=1: %s", body)
	}
}

func TestRenameDeckEndpoint(t *testing.T) {
	decks := &stubDeckRepo{}
	svc := application.Services{
		Decks:  decks,
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/decks/1", strings.NewReader(`{"name":"Renamed"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if decks.renamedID != 1 || decks.renamedName != "Renamed" {
		t.Fatalf("expected rename call, got id=%d name=%q", decks.renamedID, decks.renamedName)
	}
}

func TestGetNoteMissingReturnsNotFound(t *testing.T) {
	svc := application.Services{
		Decks:  &stubDeckRepo{},
		Notes:  stubNoteRepo{missing: true},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes/404", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["code"] != "NOTE_NOT_FOUND" || payload["recommended_action"] == "" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestRenameDeckEndpointMissingDeckReturnsNotFound(t *testing.T) {
	decks := &stubDeckRepo{missing: true}
	svc := application.Services{
		Decks:  decks,
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/decks/404", strings.NewReader(`{"name":"Renamed"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["code"] != "DECK_NOT_FOUND" || payload["recommended_action"] == "" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
	if decks.renamedID != 0 || decks.renamedName != "" {
		t.Fatalf("expected no rename call, got id=%d name=%q", decks.renamedID, decks.renamedName)
	}
}

func TestListNotesEndpoint(t *testing.T) {
	svc := application.Services{
		Decks:  &stubDeckRepo{},
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes?deck_id=1&limit=10&offset=0", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "\"items\"") || !strings.Contains(body, "\"id\":100") {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func TestCleanerDryRunStartsAsyncOperation(t *testing.T) {
	svc := application.Services{
		Decks:     &stubDeckRepo{},
		Notes:     stubNoteRepo{},
		Models:    stubModelRepo{},
		Tx:        stubTx{},
		Diff:      stubDiff{},
		Templates: sanitize.NewTemplateRegistry(),
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DefaultPageSize: 10, Workers: 1}, slog.Default(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cleaner/dry-run", strings.NewReader(`{"deck_id":1}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body=%s", rr.Code, rr.Body.String())
	}
	var accepted map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accepted body: %v", err)
	}
	operationID, ok := accepted["operation_id"].(string)
	if !ok || operationID == "" {
		t.Fatalf("expected operation_id, got %+v", accepted)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operationID, nil)
		statusRR := httptest.NewRecorder()
		handler.ServeHTTP(statusRR, statusReq)
		if statusRR.Code != http.StatusOK {
			t.Fatalf("expected status endpoint 200, got %d body=%s", statusRR.Code, statusRR.Body.String())
		}
		var state map[string]any
		if err := json.NewDecoder(statusRR.Body).Decode(&state); err != nil {
			t.Fatalf("decode status body: %v", err)
		}
		if state["status"] == "succeeded" {
			progress, ok := state["progress"].(map[string]any)
			if !ok {
				t.Fatalf("expected progress in state: %+v", state)
			}
			if progress["total"].(float64) != 1 || progress["processed"].(float64) != 1 {
				t.Fatalf("unexpected progress: %+v", progress)
			}
			return
		}
		if state["status"] == "failed" {
			t.Fatalf("operation failed: %+v", state)
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not finish: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestIndexContainsWebUIMVPControls(t *testing.T) {
	svc := application.Services{
		Decks:  &stubDeckRepo{},
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DBPath: "/tmp/collection.anki2", DefaultPageSize: 13}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Connection &amp; Settings",
		"Current Anki DB",
		"Edit DB Path",
		"toggleDBPathBtn",
		`id="dbPathEditor"`,
		"let decksLimit =  13 ;",
		"let notesLimit =  13 ;",
		"renameDeckBtn",
		"applyNoteFiltersBtn",
		"findNoteBtn",
		"globalSearchBtn",
		"Cleaner",
		"toggleCleanerBtn",
		`id="cleanerPanel" class="panel stack hidden"`,
		"previewCleanerBtn",
		"dryRunCleanerBtn",
		"applyCleanerBtn",
		"copyLogsBtn",
		"showConfigBtn",
		"saveConfigBtn",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected index to contain %q", want)
		}
	}
	if strings.Contains(body, "API base:") {
		t.Fatal("index should not show API base in settings")
	}
	if strings.Contains(body, "Connected to local API.") {
		t.Fatal("index should not include old connection success text")
	}
	if strings.Contains(body, "const notesLimit") {
		t.Fatal("notes limit should come from config, not a JS constant")
	}
	if strings.Contains(body, "const decksLimit") {
		t.Fatal("decks limit should come from config, not a JS constant")
	}
}

func TestConfigEndpointReturnsCurrentConfig(t *testing.T) {
	svc := application.Services{
		Decks:  &stubDeckRepo{},
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DBPath: "/tmp/collection.anki2", HTTPAddr: "127.0.0.1:9999", Workers: 3}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Config configSnapshot `json:"config"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode config body: %v", err)
	}
	if payload.Config.DBPath != "/tmp/collection.anki2" || payload.Config.HTTPAddr != "127.0.0.1:9999" || payload.Config.Workers != 3 {
		t.Fatalf("unexpected config payload: %+v", payload.Config)
	}
}

func TestConfigSaveWritesNextToExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	oldExecutablePath := executablePath
	executablePath = func() (string, error) {
		return filepath.Join(tmpDir, "ankiced-web-test"), nil
	}
	t.Cleanup(func() {
		executablePath = oldExecutablePath
	})

	svc := application.Services{
		Decks:  &stubDeckRepo{},
		Notes:  stubNoteRepo{},
		Models: stubModelRepo{},
	}
	handler := NewHandlerWithExit(context.Background(), svc, appconfig.Settings{DBPath: "/tmp/collection.anki2", HTTPAddr: "127.0.0.1:9999", Workers: 3}, slog.Default(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/save", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	wantPath := filepath.Join(tmpDir, "ankiced.config.json")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected saved config file at %s: %v", wantPath, err)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(data), `"db_path": "/tmp/collection.anki2"`) {
		t.Fatalf("saved config does not include db path: %s", string(data))
	}
}
