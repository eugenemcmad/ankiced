package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

func Open(url string) error {
	name, args, err := commandFor(runtime.GOOS, url)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

func commandFor(goos, url string) (string, []string, error) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	case "darwin":
		return "open", []string{url}, nil
	case "linux":
		return "xdg-open", []string{url}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform for browser open: %s", goos)
	}
}
