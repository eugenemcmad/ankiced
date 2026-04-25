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

type BackupStore struct {
	mu         sync.Mutex
	backupDone map[string]bool
}

func (s *BackupStore) CreateBackup(_ context.Context, dbPath string, now time.Time) (info domain.BackupInfo, err error) {
	s.mu.Lock()
	if s.backupDone == nil {
		s.backupDone = make(map[string]bool)
	}
	done := s.backupDone[dbPath]
	s.mu.Unlock()

	if done {
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

	s.mu.Lock()
	s.backupDone[dbPath] = true
	s.mu.Unlock()

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
