package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"ankiced/tests/testkit"
)

func TestAPIEndpointsAndLogStream(t *testing.T) {
	dbPath := createFixtureDB(t)
	addr := reserveAddr(t)
	baseURL := "http://" + addr
	binPath := testkit.BuildBinary(t, "./cmd/ankiced-web", "ankiced-e2e-test.exe")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		binPath,
		"--http-addr",
		addr,
		"--db-path",
		dbPath,
		"--force-apply",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start api: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
		}()
		select {
		case waitErr := <-waitDone:
			if waitErr != nil && !strings.Contains(waitErr.Error(), "killed") && !strings.Contains(waitErr.Error(), "exit status") {
				t.Logf("api process wait: %v", waitErr)
			}
		case <-time.After(3 * time.Second):
			if cmd.Process != nil {
				if killErr := cmd.Process.Kill(); killErr != nil {
					t.Logf("kill api process: %v", killErr)
				}
			}
			<-waitDone
		}
	})

	waitForHealth(t, baseURL+"/healthz")

	// Open SSE stream first so we can verify realtime logs.
	sseReq, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/logs/stream", nil)
	if err != nil {
		t.Fatalf("new sse request: %v", err)
	}
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("open sse stream: %v", err)
	}
	defer func() {
		if closeErr := sseResp.Body.Close(); closeErr != nil {
			t.Logf("close sse body: %v", closeErr)
		}
	}()
	if ct := sseResp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("unexpected sse content type: %q", ct)
	}

	respDecks, err := http.Get(baseURL + "/api/v1/decks")
	if err != nil {
		t.Fatalf("get decks: %v", err)
	}
	defer func() {
		if closeErr := respDecks.Body.Close(); closeErr != nil {
			t.Logf("close decks body: %v", closeErr)
		}
	}()
	if respDecks.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respDecks.Body)
		t.Fatalf("expected 200 decks, got %d body=%s", respDecks.StatusCode, string(body))
	}
	var decksPayload map[string]any
	if err := json.NewDecoder(respDecks.Body).Decode(&decksPayload); err != nil {
		t.Fatalf("decode decks: %v", err)
	}
	if _, ok := decksPayload["items"]; !ok {
		t.Fatalf("expected items field, got %+v", decksPayload)
	}

	respSearch, err := http.Get(baseURL + "/api/v1/decks/search?q=Default")
	if err != nil {
		t.Fatalf("search decks: %v", err)
	}
	defer func() {
		if closeErr := respSearch.Body.Close(); closeErr != nil {
			t.Logf("close search body: %v", closeErr)
		}
	}()
	if respSearch.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respSearch.Body)
		t.Fatalf("expected 200 search, got %d body=%s", respSearch.StatusCode, string(body))
	}

	respNotes, err := http.Get(baseURL + "/api/v1/notes?deck_id=1&limit=10&offset=0")
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	defer func() {
		if closeErr := respNotes.Body.Close(); closeErr != nil {
			t.Logf("close notes body: %v", closeErr)
		}
	}()
	if respNotes.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respNotes.Body)
		t.Fatalf("expected 200 notes, got %d body=%s", respNotes.StatusCode, string(body))
	}

	// Expect at least one log event from previous requests.
	event := readSSEEvent(t, sseResp.Body, 4*time.Second)
	if !strings.Contains(event, "\"operation\"") {
		t.Fatalf("unexpected sse event payload: %s", event)
	}

}

func reserveAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve addr: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close reserved listener: %v", err)
	}
	return addr
}

func waitForHealth(t *testing.T, healthURL string) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil {
			if resp.Body != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("api did not become healthy: %s", healthURL)
}

func readSSEEvent(t *testing.T, body io.Reader, timeout time.Duration) string {
	t.Helper()
	reader := bufio.NewReader(body)
	done := make(chan string, 1)
	go func() {
		var lines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- ""
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				if len(lines) > 0 {
					done <- strings.Join(lines, "\n")
					return
				}
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				lines = append(lines, strings.TrimPrefix(line, "data: "))
			}
		}
	}()
	select {
	case payload := <-done:
		return payload
	case <-time.After(timeout):
		t.Fatalf("timeout waiting SSE event after %s", timeout)
		return ""
	}
}


