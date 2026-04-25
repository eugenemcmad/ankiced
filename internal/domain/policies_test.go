package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDeckRename(t *testing.T) {
	if err := ValidateDeckRename(" "); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidateDeckRename("French"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateDeckRename("A\tB"); !errors.Is(err, ErrDeckNameInvalid) {
		t.Fatalf("expected invalid characters error, got %v", err)
	}
	if err := ValidateDeckRename(strings.Repeat("a", 201)); !errors.Is(err, ErrDeckNameTooLong) {
		t.Fatalf("expected deck name too long, got %v", err)
	}
}

func TestMapFields(t *testing.T) {
	model := Model{ID: 1, FieldNames: []string{"Front", "Back"}}
	fields, err := MapFields([]string{"q", "a"}, model)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(fields) != 2 || fields[0].Name != "Front" || fields[1].Value != "a" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
}
