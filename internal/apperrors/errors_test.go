package apperrors

import (
	"fmt"
	"testing"
)

func TestHasCodeFromWrappedError(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", New(CodeDeckNotFound, "deck not found"))
	if !HasCode(err, CodeDeckNotFound) {
		t.Fatal("expected CodeDeckNotFound")
	}
	if HasCode(err, CodeModelNotFound) {
		t.Fatal("did not expect CodeModelNotFound")
	}
}

func TestAppErrorErrorIncludesCauseAndMessage(t *testing.T) {
	cause := fmt.Errorf("disk full")
	err := Wrap(CodeReportWriteFailed, "report write failed", cause)
	got := err.Error()
	want := "report write failed: disk full"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestAppErrorErrorFallsBackToCode(t *testing.T) {
	err := &AppError{Code: CodeOperationCancelled}
	if got := err.Error(); got != "OPERATION_CANCELLED" {
		t.Fatalf("expected fallback to code, got %q", got)
	}
}

func TestAppErrorErrorReturnsCauseWhenMessageEmpty(t *testing.T) {
	cause := fmt.Errorf("io error")
	err := &AppError{Code: CodeReportWriteFailed, Cause: cause}
	if got := err.Error(); got != "io error" {
		t.Fatalf("expected cause text, got %q", got)
	}
}

func TestAppErrorErrorNilSafe(t *testing.T) {
	var err *AppError
	if got := err.Error(); got != "" {
		t.Fatalf("expected empty for nil receiver, got %q", got)
	}
}
