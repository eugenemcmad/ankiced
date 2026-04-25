package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

func withJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		next.ServeHTTP(w, r)
	})
}

func (h *apiHandler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil && h.logger != nil {
		h.logger.Warn("write json response failed", "status", status, "error", err)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func parsePathID(path, prefix string) (int64, error) {
	if !strings.HasPrefix(path, prefix) {
		return 0, errors.New("invalid prefix")
	}
	idPart := strings.TrimPrefix(path, prefix)
	if strings.Contains(idPart, "/") {
		return 0, errors.New("invalid nested path")
	}
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func parseInt64Default(v string, fallback int64) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func appConfigPath() (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	dir := filepath.Dir(exe)
	if strings.TrimSpace(dir) == "" || dir == "." {
		return "", errors.New("executable directory is not available")
	}
	return filepath.Join(dir, "ankiced.config.json"), nil
}

type configSnapshot struct {
	DBPath            string `json:"db_path"`
	AnkiAccount       string `json:"anki_account"`
	HTTPAddr          string `json:"http_addr"`
	BackupKeepLastN   int    `json:"backup_keep_last_n"`
	Workers           int    `json:"workers"`
	ForceApply        bool   `json:"force_apply"`
	Verbose           bool   `json:"verbose"`
	FullDiff          bool   `json:"full_diff"`
	ReportFile        string `json:"report_file"`
	DefaultPageSize   int    `json:"default_page_size"`
	PragmaBusyTimeout int    `json:"pragma_busy_timeout"`
	PragmaJournalMode string `json:"pragma_journal_mode"`
	PragmaSynchronous string `json:"pragma_synchronous"`
}
