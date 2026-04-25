package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

type Prompter struct {
	In  io.Reader
	Out io.Writer
}

func (p Prompter) Confirm(_ context.Context, prompt string) (bool, error) {
	if _, err := fmt.Fprintf(p.Out, "%s [y/N]: ", prompt); err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(p.In)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes", nil
}
