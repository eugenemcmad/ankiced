package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ankiced/internal/domain"
)

// BackupStore creates the per-session SQLite backup at most once per database
// path. Concurrency is enforced via a per-path *sync.Mutex so two goroutines
// cannot race past the "already done" check while still allowing different
// databases to back up in parallel.
type BackupStore struct {
	mu    sync.Mutex
	locks map[string]*pathLock
}

type pathLock struct {
	mu   sync.Mutex
	done bool
}

func (s *BackupStore) lockFor(path string) *pathLock {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks == nil {
		s.locks = make(map[string]*pathLock)
	}
	pl, ok := s.locks[path]
	if !ok {
		pl = &pathLock{}
		s.locks[path] = pl
	}
	return pl
}

func (s *BackupStore) CreateBackup(_ context.Context, dbPath string, now time.Time) (info domain.BackupInfo, err error) {
	pl := s.lockFor(dbPath)
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.done {
		return domain.BackupInfo{}, nil
	}

	src, err := os.Open(dbPath)
	if err != nil {
		return domain.BackupInfo{}, err
	}
	defer func() {
		if closeErr := src.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close source file: %w", closeErr))
		}
	}()

	stamp := now.UTC().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.%s.bak", dbPath, stamp)
	dst, err := os.Create(backupPath)
	if err != nil {
		return domain.BackupInfo{}, err
	}
	defer func() {
		if syncErr := dst.Sync(); syncErr != nil {
			err = errors.Join(err, fmt.Errorf("sync backup file: %w", syncErr))
		}
		if closeErr := dst.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close backup file: %w", closeErr))
		}
	}()

	if _, err := io.Copy(dst, src); err != nil {
		return domain.BackupInfo{}, err
	}
	info = domain.BackupInfo{Path: backupPath, CreatedAt: now}
	pl.done = true
	return info, nil
}

func (s *BackupStore) CleanupBackups(_ context.Context, dbPath string, keepLastN int) error {
	if keepLastN < 1 {
		keepLastN = 3
	}
	dir := filepath.Dir(dbPath)
	base := filepath.Base(dbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var backups []string
	prefix := base + "."
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".bak") {
			backups = append(backups, filepath.Join(dir, name))
		}
	}
	sort.Strings(backups)
	if len(backups) <= keepLastN {
		return nil
	}
	for _, p := range backups[:len(backups)-keepLastN] {
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	return nil
}
