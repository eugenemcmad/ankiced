package browser

import "testing"

func TestCommandFor(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		url     string
		cmd     string
		argsLen int
		wantErr bool
	}{
		{name: "windows", goos: "windows", url: "http://127.0.0.1:8080", cmd: "rundll32", argsLen: 2},
		{name: "darwin", goos: "darwin", url: "http://127.0.0.1:8080", cmd: "open", argsLen: 1},
		{name: "linux", goos: "linux", url: "http://127.0.0.1:8080", cmd: "xdg-open", argsLen: 1},
		{name: "unsupported", goos: "plan9", url: "http://127.0.0.1:8080", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, args, err := commandFor(tc.goos, tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s", tc.goos)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd != tc.cmd {
				t.Fatalf("expected command %q, got %q", tc.cmd, cmd)
			}
			if len(args) != tc.argsLen {
				t.Fatalf("expected %d args, got %d", tc.argsLen, len(args))
			}
		})
	}
}
