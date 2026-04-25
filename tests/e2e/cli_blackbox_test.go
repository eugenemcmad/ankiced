package e2e

import (
	"context"
	"database/sql"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ankiced/tests/testkit"
	_ "modernc.org/sqlite"
)

func TestCLIBlackBoxFlow(t *testing.T) {
	dbPath := createFixtureDB(t)
	binPath := testkit.BuildBinary(t, "./cmd/ankiced", "ankiced-e2e-cli.exe")
	cmd := exec.CommandContext(
		context.Background(),
		binPath,
		"--db-path",
		dbPath,
		"--force-apply",
	)
	cmd.Dir = testkit.RepoRoot(t)
	cmd.Stdin = strings.NewReader("1\n0\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run cli failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "id=1 name=Default") {
		t.Fatalf("unexpected output:\n%s", string(out))
	}
}

func TestCLIEditNoteDecodesEscapes(t *testing.T) {
	dbPath := createFixtureDB(t)
	binPath := testkit.BuildBinary(t, "./cmd/ankiced", "ankiced-e2e-cli.exe")
	cmd := exec.CommandContext(
		context.Background(),
		binPath,
		"--db-path",
		dbPath,
		"--force-apply",
	)
	cmd.Dir = testkit.RepoRoot(t)
	cmd.Stdin = strings.NewReader("4\n100\nline\\nnext\n.end\nback\\tvalue\n.end\n0\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run cli failed: %v\n%s", err, string(out))
	}

	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close verification db: %v", closeErr)
		}
	})
	var flds string
	if err := db.QueryRow(`SELECT flds FROM notes WHERE id = 100`).Scan(&flds); err != nil {
		t.Fatalf("read updated note: %v", err)
	}
	if flds != "line\nnext\x1fback\tvalue" {
		t.Fatalf("unexpected flds %q", flds)
	}
}

func TestCLIVerbosePrintsDebugCauseForInputError(t *testing.T) {
	dbPath := createFixtureDB(t)
	binPath := testkit.BuildBinary(t, "./cmd/ankiced", "ankiced-e2e-cli.exe")
	cmd := exec.CommandContext(
		context.Background(),
		binPath,
		"--db-path",
		dbPath,
		"--force-apply",
		"--verbose",
	)
	cmd.Dir = testkit.RepoRoot(t)
	cmd.Stdin = strings.NewReader("4\n100\nbad\\x\n0\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run cli failed: %v\n%s", err, string(out))
	}
	output := string(out)
	if !strings.Contains(output, "error: invalid escape sequence in multiline input | cause:") {
		t.Fatalf("unexpected verbose output:\n%s", output)
	}
}

func createFixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collection.anki2")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close fixture db: %v", closeErr)
		}
	})
	testkit.InitSchema(t, db)
	testkit.SeedData(t, db)
	return path
}


