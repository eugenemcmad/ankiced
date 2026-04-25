package testkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	builtBinaries sync.Map
)

func BuildBinary(t *testing.T, targetPath, binName string) string {
	t.Helper()
	repo := RepoRoot(t)
	binDir := filepath.Join(repo, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, binName)

	if _, ok := builtBinaries.Load(binName); ok {
		return binPath
	}

	buildCmd := exec.Command("go", "build", "-o", binPath, targetPath)
	buildCmd.Dir = repo
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build binary %s: %v\n%s", binName, err, string(out))
	}

	builtBinaries.Store(binName, true)

	return binPath
}

func RepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Depending on where go test is running, find the root.
	// e2e tests run in tests/e2e
	if filepath.Base(wd) == "e2e" || filepath.Base(wd) == "integration" {
		return filepath.Clean(filepath.Join(wd, "..", ".."))
	}
	return wd
}
