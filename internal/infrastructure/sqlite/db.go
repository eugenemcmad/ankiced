package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

type DB struct {
	mu sync.RWMutex
	// reconnectMu serialises Reconnect calls so two concurrent reconnects
	// can never race on closing each other's "old" handle. It is held for
	// the entire reconnect (open/ping/swap/close) and intentionally
	// distinct from `mu` so read paths are not blocked while a slow
	// Close() drains.
	reconnectMu sync.Mutex
	SQL         *sql.DB
	pragmas     Pragmas
}

func (d *DB) Conn() *sql.DB {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.SQL
}

// Pragmas captures the SQLite PRAGMA values applied at connection open.
// Empty/zero fields fall back to safe defaults.
type Pragmas struct {
	BusyTimeoutMS int
	JournalMode   string
	Synchronous   string
}

var (
	allowedJournalModes = map[string]struct{}{
		"WAL": {}, "DELETE": {}, "TRUNCATE": {}, "MEMORY": {}, "OFF": {}, "PERSIST": {},
	}
	allowedSynchronous = map[string]struct{}{
		"OFF": {}, "NORMAL": {}, "FULL": {}, "EXTRA": {},
	}
)

// ErrInvalidJournalMode is returned when an unsupported journal_mode value is
// supplied via configuration. Whitelist prevents PRAGMA injection through the DSN.
var ErrInvalidJournalMode = errors.New("invalid journal_mode pragma")

// ErrInvalidSynchronous is returned when an unsupported synchronous value is
// supplied via configuration.
var ErrInvalidSynchronous = errors.New("invalid synchronous pragma")

// ErrOldConnCloseFailed is returned by Reconnect when the new connection has
// already replaced the old one but closing the previous handle failed. The
// reconnect itself is effectively successful and callers may treat this as a
// warning.
var ErrOldConnCloseFailed = errors.New("close previous db connection failed")

func (p Pragmas) normalize() (Pragmas, error) {
	out := p
	if out.BusyTimeoutMS <= 0 {
		out.BusyTimeoutMS = 5000
	}
	mode := strings.ToUpper(strings.TrimSpace(out.JournalMode))
	if mode == "" {
		mode = "WAL"
	}
	if _, ok := allowedJournalModes[mode]; !ok {
		return Pragmas{}, fmt.Errorf("%w: %q", ErrInvalidJournalMode, p.JournalMode)
	}
	out.JournalMode = mode
	syncMode := strings.ToUpper(strings.TrimSpace(out.Synchronous))
	if syncMode == "" {
		syncMode = "NORMAL"
	}
	if _, ok := allowedSynchronous[syncMode]; !ok {
		return Pragmas{}, fmt.Errorf("%w: %q", ErrInvalidSynchronous, p.Synchronous)
	}
	out.Synchronous = syncMode
	return out, nil
}

// dsnPathEscaper escapes only the characters that interact with the SQLite
// URI syntax: '%' (escape introducer), '?' (query separator), '#' (fragment
// separator) and SP. Other URI-reserved characters such as '/', ':' and '\'
// are preserved verbatim so Windows-style paths (e.g. `C:\Users\me\db`)
// still round-trip through the modernc.org/sqlite driver.
var dsnPathEscaper = strings.NewReplacer(
	"%", "%25",
	"?", "%3F",
	"#", "%23",
	" ", "%20",
)

// buildDSN renders a SQLite URI DSN with PRAGMA hints. The path component is
// minimally escaped so user-controlled paths (which may legally contain '?'
// or '#') cannot be reinterpreted as query/fragment delimiters by the
// driver, while leaving conventional path separators intact.
func buildDSN(path string, p Pragmas) string {
	return fmt.Sprintf(
		"file:%s?_pragma=journal_mode(%s)&_pragma=synchronous(%s)&_pragma=busy_timeout(%d)&_pragma=foreign_keys(ON)",
		dsnPathEscaper.Replace(path), p.JournalMode, p.Synchronous, p.BusyTimeoutMS,
	)
}

// Open opens a SQLite database with the provided pragmas. Journal mode and
// synchronous values are validated against an internal whitelist to prevent
// PRAGMA-injection through DSN string interpolation.
func Open(path string, p Pragmas) (*DB, error) {
	pragmas, err := p.normalize()
	if err != nil {
		return nil, err
	}
	dsn := buildDSN(path, pragmas)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close db after ping failure: %w", closeErr))
		}
		return nil, err
	}
	return &DB{SQL: db, pragmas: pragmas}, nil
}

// Reconnect swaps the underlying *sql.DB for one pointing at `path`. The new
// connection is fully validated (Ping) before the old one is replaced. If
// closing the old handle fails after a successful swap, the error is returned
// so the caller (e.g. the HTTP handler) can log it; reconnection itself is
// considered successful in that case because new requests will use newDB.
//
// Concurrent Reconnect calls are serialized by reconnectMu so two callers can
// never race on closing each other's "old" handle (which would either
// double-close the same *sql.DB or close a handle that has already been
// adopted by a different caller).
func (d *DB) Reconnect(path string) error {
	d.reconnectMu.Lock()
	defer d.reconnectMu.Unlock()

	dsn := buildDSN(path, d.pragmas)
	newDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	newDB.SetMaxOpenConns(1)
	newDB.SetMaxIdleConns(1)
	if err := newDB.Ping(); err != nil {
		if closeErr := newDB.Close(); closeErr != nil {
			return errors.Join(err, fmt.Errorf("close new db after ping failure: %w", closeErr))
		}
		return err
	}

	d.mu.Lock()
	oldDB := d.SQL
	d.SQL = newDB
	d.mu.Unlock()

	if oldDB != nil {
		if closeErr := oldDB.Close(); closeErr != nil {
			return fmt.Errorf("%w: %v", ErrOldConnCloseFailed, closeErr)
		}
	}
	return nil
}

func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.SQL != nil {
		return d.SQL.Close()
	}
	return nil
}

type txKey struct{}

func withTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

func txFrom(ctx context.Context) *sql.Tx {
	tx, ok := ctx.Value(txKey{}).(*sql.Tx)
	if !ok {
		return nil
	}
	return tx
}

func (d *DB) WithTx(ctx context.Context, fn func(context.Context) error) error {
	conn := d.Conn()
	if conn == nil {
		return errors.New("db not connected")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txCtx := withTx(ctx, tx)
	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rbErr))
		}
		return err
	}
	return tx.Commit()
}

func queryer(ctx context.Context, d *DB) interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} {
	if tx := txFrom(ctx); tx != nil {
		return tx
	}
	return d.Conn()
}
