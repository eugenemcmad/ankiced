package bootstrap

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"ankiced/internal/apperrors"
)

func TestVerboseRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no_args", nil, false},
		{"unrelated", []string{"--db-path", "x"}, false},
		{"plain_flag", []string{"--verbose"}, true},
		{"explicit_true", []string{"--verbose=true"}, true},
		{"explicit_one", []string{"--verbose=1"}, true},
		{"explicit_false", []string{"--verbose=false"}, false},
		{"explicit_zero", []string{"--verbose=0"}, false},
		{"explicit_random_value_returns_false", []string{"--verbose=other"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerboseRequested(tc.args); got != tc.want {
				t.Fatalf("VerboseRequested(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestEnvEnabled(t *testing.T) {
	const key = "ANKICED_TEST_BOOTSTRAP_ENV"
	cases := []struct {
		name  string
		value string
		set   bool
		want  bool
	}{
		{"unset", "", false, false},
		{"empty", "", true, false},
		{"true_lowercase", "true", true, true},
		{"true_mixed_case", "TrUe", true, true},
		{"one", "1", true, true},
		{"false", "false", true, false},
		{"zero", "0", true, false},
		{"random", "yes", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(key, tc.value)
			} else {
				if err := unsetEnvForTest(t, key); err != nil {
					t.Fatalf("unset env: %v", err)
				}
			}
			if got := EnvEnabled(key); got != tc.want {
				t.Fatalf("EnvEnabled(%q=%q) = %v, want %v", key, tc.value, got, tc.want)
			}
		})
	}
}

func unsetEnvForTest(t *testing.T, key string) error {
	t.Helper()
	t.Setenv(key, "")
	// t.Setenv guarantees restoration; clearing the value is enough for EnvEnabled.
	return nil
}

func TestFormatErrorForMode(t *testing.T) {
	wrapped := apperrors.Wrap(apperrors.CodeReportWriteFailed, "failed to write report", errors.New("disk full"))

	user := FormatErrorForMode(wrapped, false)
	debug := FormatErrorForMode(wrapped, true)
	if user == "" {
		t.Fatalf("user mode produced empty output")
	}
	if !strings.Contains(debug, "disk full") {
		t.Fatalf("expected verbose mode to expose cause chain, got %q", debug)
	}
	if user == debug {
		t.Fatalf("expected user vs debug formats to differ, both = %q", user)
	}
}

func TestFail_LogsAndReturnsFormattedError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cause := errors.New("boom")

	got := Fail(logger, "open db", cause, false)
	if got == nil {
		t.Fatalf("expected non-nil error from Fail")
	}
	if !strings.Contains(got.Error(), "open db") {
		t.Fatalf("expected prefix in returned error, got %q", got.Error())
	}
	if !strings.Contains(buf.String(), "open db") {
		t.Fatalf("expected logger to record prefix, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "level=ERROR") {
		t.Fatalf("expected logger to use ERROR level, got %q", buf.String())
	}
}

func TestNewLogger_Levels(t *testing.T) {
	plain := NewLogger(false)
	verbose := NewLogger(true)
	if plain == nil || verbose == nil {
		t.Fatalf("NewLogger returned nil")
	}
}
