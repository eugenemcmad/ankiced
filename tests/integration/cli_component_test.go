package integration

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appconfig "ankiced/internal/config"
	"ankiced/internal/interfaces/cli"
)

func TestInteractiveMenuFlowComponent(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})

	svc := setupTestServices(db, cli.Prompter{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}})

	in := strings.NewReader("1\n5\n100\n0\n")
	var out bytes.Buffer
	app := cli.App{Svc: svc, Cfg: appconfig.Settings{ForceApply: true, Workers: 1}, In: in, Out: &out}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("app run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "id=1 name=Default") {
		t.Fatalf("expected deck list in output: %s", got)
	}
}

func TestCLIFindNoteByIDShowsPreview(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})

	svc := setupTestServices(db, cli.Prompter{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}})

	in := strings.NewReader("8\n100\n0\n")
	var out bytes.Buffer
	app := cli.App{Svc: svc, Cfg: appconfig.Settings{ForceApply: true, Workers: 1}, In: in, Out: &out}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("app run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "note=100") {
		t.Fatalf("expected note line in output: %s", got)
	}
}

func TestCLISearchDecksByText(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})
	if _, err := db.SQL.Exec(`INSERT INTO decks(id, name) VALUES (2, 'Filtered Deck')`); err != nil {
		t.Fatalf("insert filtered deck: %v", err)
	}

	svc := setupTestServices(db, cli.Prompter{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}})

	in := strings.NewReader("10\nFiltered\n0\n")
	var out bytes.Buffer
	app := cli.App{Svc: svc, Cfg: appconfig.Settings{ForceApply: true, Workers: 1}, In: in, Out: &out}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("app run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "id=2 name=Filtered Deck") {
		t.Fatalf("expected filtered deck in output: %s", got)
	}
}

func TestCLIShowsValidationErrorAndContinues(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})

	svc := setupTestServices(db, cli.Prompter{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}})

	in := strings.NewReader("4\n100\nbad\\x\n0\n")
	var out bytes.Buffer
	app := cli.App{Svc: svc, Cfg: appconfig.Settings{ForceApply: true, Workers: 1}, In: in, Out: &out}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("app run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "error: invalid escape sequence in multiline input") {
		t.Fatalf("expected invalid escape message, got: %s", got)
	}
	if strings.Count(got, "== Ankiced ==") < 2 {
		t.Fatalf("expected app to continue loop, got: %s", got)
	}
}

func TestCLICancelledOperationDoesNotCrash(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})

	svc := setupTestServices(db, cli.Prompter{In: strings.NewReader("n\n"), Out: &bytes.Buffer{}})

	in := strings.NewReader("7\n1\n0\n")
	var out bytes.Buffer
	app := cli.App{Svc: svc, Cfg: appconfig.Settings{ForceApply: false, Workers: 1}, In: in, Out: &out}
	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("app run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "error: operation cancelled by user") {
		t.Fatalf("expected cancellation message, got: %s", got)
	}
	if strings.Count(got, "== Ankiced ==") < 2 {
		t.Fatalf("expected app to continue loop, got: %s", got)
	}
}

func TestAppRunStopsOnContextCancel(t *testing.T) {
	_, db := createFixtureDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("close fixture db: %v", err)
		}
	})

	svc := setupTestServices(db, cli.Prompter{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := strings.NewReader("1\n0\n")
	var out bytes.Buffer
	app := cli.App{Svc: svc, Cfg: appconfig.Settings{ForceApply: true, Workers: 1}, In: in, Out: &out}

	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("app did not stop on context cancellation")
	}
}
