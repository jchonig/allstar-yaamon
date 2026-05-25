package yaamon

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func promptPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	if len(b) == 0 {
		return "", fmt.Errorf("password cannot be empty")
	}
	return string(b), nil
}
